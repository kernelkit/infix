"""Flatten an OCI image into a plain rootfs directory.

Input is an OCI image archive (.tar or .tar.gz) or an OCI layout
directory.  Output is <dst>/rootfs, <dst>/config.json (the image config
blob, verbatim) and <dst>/digest (the config digest, used as image ID).
Layers are applied in order with whiteouts resolved between them, so the
result is exactly what the container sees as its root and can be used
read-only as an overlay lower directly from a squashfs.

Runs standalone on the build host as well as on the target:

    python3 oci.py flatten IMAGE.tar.gz /lib/oci/name
"""
import argparse
import copy
import json
import os
import platform
import re
import shutil
import sys
import tarfile
import tempfile

WHITEOUT = ".wh."
OPAQUE = ".wh..wh..opq"

ARCH = {
    "x86_64": "amd64",
    "aarch64": "arm64",
    "riscv64": "riscv64",
    "armv7l": "arm",
}


class ImageError(Exception):
    pass


def host_arch():
    return ARCH.get(platform.machine(), platform.machine())


def _blob(layout, digest):
    algo, _, hexd = digest.partition(":")
    if not re.fullmatch(r"[a-z0-9]+", algo) or not re.fullmatch(r"[a-f0-9]+", hexd):
        raise ImageError(f"malformed digest {digest}")
    return os.path.join(layout, "blobs", algo, hexd)


def _find_layout(root):
    for dirpath, dirs, files in os.walk(root):
        if "index.json" in files and "blobs" in dirs:
            return dirpath
    raise ImageError(f"no OCI layout (index.json) found under {root}")


def _read_json(path):
    with open(path) as f:
        return json.load(f)


def _select(manifests, arch):
    for desc in manifests:
        plat = desc.get("platform")
        if not plat or (plat.get("os", "linux") == "linux" and plat.get("architecture") == arch):
            return desc
    raise ImageError(f"no manifest for linux/{arch}")


def _manifest(layout, arch):
    index = _read_json(os.path.join(layout, "index.json"))
    desc = _select(index.get("manifests", []), arch)
    doc = _read_json(_blob(layout, desc["digest"]))
    if "manifests" in doc:
        desc = _select(doc["manifests"], arch)
        doc = _read_json(_blob(layout, desc["digest"]))
    return doc


def inside(rootfs, relpath, follow=False):
    """Path of relpath in rootfs, with symlinks resolved as the container
    sees them: an absolute target or a .. never leads out of rootfs.  The
    last component is only resolved when follow is set."""
    todo = [p for p in reversed(relpath.split("/")) if p not in ("", ".")]
    done = []
    links = 0
    while todo:
        part = todo.pop()
        if part == "..":
            del done[-1:]
            continue
        path = os.path.join(rootfs, *done, part)
        if (todo or follow) and os.path.islink(path):
            links += 1
            if links > 40:
                raise ImageError(f"too many levels of symbolic links: {relpath}")
            target = os.readlink(path)
            if target.startswith("/"):
                done = []
            todo += [p for p in reversed(target.split("/")) if p not in ("", ".")]
            continue
        done.append(part)
    return os.path.join(rootfs, *done)


def _remove(path):
    if os.path.islink(path) or os.path.isfile(path):
        os.unlink(path)
    elif os.path.isdir(path):
        shutil.rmtree(path)
    elif os.path.lexists(path):
        os.unlink(path)


def _whiteouts(members, rootfs):
    for m in members:
        base = os.path.basename(m.name)
        if not base.startswith(WHITEOUT):
            continue
        parent = inside(rootfs, os.path.dirname(m.name), follow=True)
        if base == OPAQUE:
            if os.path.isdir(parent):
                for entry in os.listdir(parent):
                    _remove(os.path.join(parent, entry))
        elif base != WHITEOUT:
            _remove(os.path.join(parent, base[len(WHITEOUT):]))


def _apply_layer(blob, rootfs):
    with tarfile.open(blob, "r:*") as tar:
        members = tar.getmembers()
        _whiteouts(members, rootfs)
        dirs = []
        for m in members:
            if os.path.basename(m.name).startswith(WHITEOUT):
                continue
            target = inside(rootfs, m.name)
            # Anything but a directory over a directory replaces what is there
            if os.path.lexists(target) and not (m.isdir() and os.path.isdir(target)
                                                and not os.path.islink(target)):
                _remove(target)
            m = copy.copy(m)
            m.name = os.path.relpath(target, rootfs)
            if m.islnk():
                m.linkname = os.path.relpath(inside(rootfs, m.linkname), rootfs)
            tar.extract(m, rootfs, set_attrs=not m.isdir(), numeric_owner=True,
                        filter="fully_trusted")
            if m.isdir():
                dirs.append(m)

        # Directory attributes last, a read-only one can then be populated
        for m in reversed(dirs):
            path = os.path.join(rootfs, m.name)
            if os.path.isdir(path) and not os.path.islink(path):
                tar.chown(m, path, True)
                tar.utime(m, path)
                tar.chmod(m, path)


def flatten(src, dst, arch=None):
    """Flatten image archive or layout at src into dst, return (config, digest)."""
    arch = arch or host_arch()
    dst = os.path.abspath(dst)
    os.makedirs(os.path.dirname(dst), exist_ok=True)
    tmp = None
    try:
        if os.path.isdir(src):
            layout = _find_layout(src)
        else:
            tmp = tempfile.mkdtemp(prefix=".flatten-", dir=os.path.dirname(dst))
            with tarfile.open(src, "r:*") as tar:
                # Blobs and an index, nothing else belongs in an archive
                tar.extractall(tmp, filter="data")
            layout = _find_layout(tmp)

        manifest = _manifest(layout, arch)
        digest = manifest["config"]["digest"]
        config = _read_json(_blob(layout, digest))

        staging = dst + ".tmp"
        _remove(staging)
        rootfs = os.path.join(staging, "rootfs")
        os.makedirs(rootfs)
        for layer in manifest.get("layers", []):
            if "zstd" in layer.get("mediaType", ""):
                raise ImageError("zstd compressed layers are not supported")
            _apply_layer(_blob(layout, layer["digest"]), rootfs)

        shutil.copyfile(_blob(layout, digest), os.path.join(staging, "config.json"))
        with open(os.path.join(staging, "digest"), "w") as f:
            f.write(digest.split(":", 1)[1] + "\n")

        _remove(dst)
        os.rename(staging, dst)
    finally:
        if tmp:
            shutil.rmtree(tmp, ignore_errors=True)
        _remove(dst + ".tmp")

    return config, digest.split(":", 1)[1]


def load_layout(path, arch=None):
    """Return (config, digest) of an OCI layout without flattening it."""
    layout = _find_layout(path)
    manifest = _manifest(layout, arch or host_arch())
    digest = manifest["config"]["digest"]
    return _read_json(_blob(layout, digest)), digest.split(":", 1)[1]


def is_flat(path):
    return os.path.isdir(os.path.join(path, "rootfs")) and os.path.isfile(os.path.join(path, "config.json"))


def load_flat(path):
    """Return (config, digest) of a flattened image directory."""
    config = _read_json(os.path.join(path, "config.json"))
    with open(os.path.join(path, "digest")) as f:
        return config, f.read().strip()


def main(argv=None):
    parser = argparse.ArgumentParser(description="Flatten an OCI image to a rootfs directory")
    sub = parser.add_subparsers(dest="cmd", required=True)
    fl = sub.add_parser("flatten")
    fl.add_argument("src", help="OCI archive (.tar, .tar.gz) or layout directory")
    fl.add_argument("dst", help="output directory")
    fl.add_argument("--arch", default=None, help="target architecture, e.g. amd64 (default: host)")
    args = parser.parse_args(argv)

    try:
        _, digest = flatten(args.src, args.dst, args.arch)
    except (ImageError, OSError, tarfile.TarError, KeyError) as e:
        print(f"oci: {e}", file=sys.stderr)
        return 1
    print(digest)
    return 0


if __name__ == "__main__":
    sys.exit(main())
