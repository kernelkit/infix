"""Convert an OCI image to a SIF image.

Input is an OCI image archive (.tar or .tar.gz) or an OCI layout
directory.  The layers are merged, with whiteouts resolved, into one
tar stream that sqfstar turns into the SquashFS of the SIF file.  The
image is never unpacked: nothing of it touches the file system here, so
a layer cannot write outside of it, and owners, modes and devices
survive a conversion done by a regular user at build time.

Runs on the build host as well as on the target:

    python3 -m ixcont.oci convert IMAGE.tar.gz /lib/oci/name.sif
"""
import argparse
import copy
import json
import os
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile

from . import sif

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


# --- merging the layers ---------------------------------------------------

def _norm(path):
    parts = [p for p in path.split("/") if p not in ("", ".")]
    if ".." in parts:
        raise ImageError(f"layer entry outside the image: {path}")
    return "/".join(parts)


def _name(member):
    return _norm(member.name)


def _parents(name):
    while name:
        name = os.path.dirname(name)
        yield name


def _pick(layers):
    """The entries of the image: per path that of the topmost layer that
    has it, unless a layer above removed it.  Maps path to (layer, member)."""
    picked = {}
    removed, emptied = set(), set()         # by the layers above this one
    for idx in reversed(range(len(layers))):
        rm, empty = set(), set()
        for m in reversed(layers[idx].getmembers()):
            name = _name(m)
            parent, base = os.path.split(name)
            if base == OPAQUE:
                empty.add(parent)
            elif base.startswith(WHITEOUT):
                rm.add(os.path.join(parent, base[len(WHITEOUT):]))
            elif name in picked or name in removed or not removed.isdisjoint(_parents(name)) \
                    or not emptied.isdisjoint(_parents(name)):
                continue
            else:
                picked[name] = (idx, m)
                if not m.isdir():
                    empty.add(name)
        removed |= rm
        emptied |= empty

    for name, (_, m) in picked.items():
        for parent in _parents(name):
            if parent in picked and not picked[parent][1].isdir():
                raise ImageError(f"layer entry below something not a directory: {name}")
    return picked


def _entry(member, name):
    """A copy of member to write as name, with only its extended attributes
    kept of the pax headers, the rest would override what is set here."""
    m = copy.copy(member)
    m.name = name
    m.pax_headers = {k: v for k, v in member.pax_headers.items() if ".xattr." in k}
    return m


def _linked(names, member):
    """The file a hard link refers to, names being the entries of its layer."""
    for _ in range(8):
        member = names.get(_norm(member.linkname))
        if member is None:
            break
        if not member.islnk():
            return member
    raise ImageError("hard link without a file in its layer")


def _write(layers, picked, out):
    """Write the picked entries as one tar: directories, then the rest
    layer by layer, and last the hard links, which need their files."""
    for name in sorted(n for n, (_, m) in picked.items() if n and m.isdir()):
        out.addfile(_entry(picked[name][1], name))

    links = []
    for idx, layer in enumerate(layers):
        names = {_name(m): m for m in layer.getmembers()}
        for m in layer.getmembers():
            name = _name(m)
            if picked.get(name) != (idx, m) or m.isdir():
                continue
            if m.islnk():
                # The file may be another in the image than in the layer
                file = _linked(names, m)
                if picked.get(_name(file)) == (idx, file):
                    link = _entry(m, name)
                    link.linkname = _name(file)
                    links.append(link)
                    continue
                entry = _entry(file, name)
            else:
                file, entry = m, _entry(m, name)

            if file.isreg():
                entry.type = tarfile.REGTYPE
                out.addfile(entry, layer.extractfile(file))
            else:
                out.addfile(entry)
    for link in links:
        out.addfile(link)


def _squash(layers, dst):
    """Merge the layers into a SquashFS in dst, return the time of its newest entry."""
    picked = _pick(layers)
    root = picked[""][1] if "" in picked else tarfile.TarInfo()
    newest = int(max((m.mtime for _, m in picked.values()), default=0))

    cmd = ["sqfstar", "-quiet", "-no-progress",
           "-offset", str(sif.PARTITION_OFFSET),
           "-mkfs-time", str(newest), "-root-time", str(newest),
           "-default-mode", "0755", "-default-uid", "0", "-default-gid", "0",
           "-root-mode", "%o" % (root.mode & 0o7777 if "" in picked else 0o755),
           "-root-uid", str(root.uid), "-root-gid", str(root.gid), dst]
    proc = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.DEVNULL,
                            stderr=subprocess.PIPE)
    try:
        with tarfile.open(fileobj=proc.stdin, mode="w|", format=tarfile.PAX_FORMAT) as out:
            _write(layers, picked, out)
        proc.stdin.close()
    except BrokenPipeError:
        pass
    err = proc.stderr.read().decode(errors="replace").strip()
    if proc.wait():
        raise ImageError(err.splitlines()[0] if err else "sqfstar failed")
    return newest


def convert(src, dst, arch=None, source=None):
    """Convert image archive or layout at src to the SIF dst, return its config.

    source is where the image came from, to keep with it.
    """
    arch = arch or host_arch()
    dst = os.path.abspath(dst)
    os.makedirs(os.path.dirname(dst), exist_ok=True)
    tmp = dst + ".tmp"
    if os.path.exists(tmp):
        os.unlink(tmp)
    unpacked = None
    layers = []
    try:
        if os.path.isdir(src):
            layout = _find_layout(src)
        else:
            unpacked = tempfile.mkdtemp(prefix=".layout-", dir=os.path.dirname(dst))
            with tarfile.open(src, "r:*") as tar:
                # Blobs and an index, nothing else belongs in an archive
                tar.extractall(unpacked, filter="data")
            layout = _find_layout(unpacked)

        manifest = _manifest(layout, arch)
        digest = manifest["config"]["digest"]
        config = _read_json(_blob(layout, digest))
        for layer in manifest.get("layers", []):
            if "zstd" in layer.get("mediaType", ""):
                raise ImageError("zstd compressed layers are not supported")
            layers.append(tarfile.open(_blob(layout, layer["digest"]), "r:*"))

        newest = _squash(layers, tmp)
        hexd = digest.split(":", 1)[1]
        sif.wrap(tmp, arch, bytes.fromhex(hexd[:32]), newest, {
            sif.CONFIG: config,
            sif.SOURCE: {"ref": source or os.path.basename(src), "digest": hexd},
        })
        os.replace(tmp, dst)
    finally:
        for layer in layers:
            layer.close()
        if unpacked:
            shutil.rmtree(unpacked, ignore_errors=True)
        if os.path.exists(tmp):
            os.unlink(tmp)

    return config


def main(argv=None):
    parser = argparse.ArgumentParser(description="Convert an OCI image to a SIF image")
    sub = parser.add_subparsers(dest="cmd", required=True)
    cv = sub.add_parser("convert")
    cv.add_argument("src", help="OCI archive (.tar, .tar.gz) or layout directory")
    cv.add_argument("dst", help="SIF image to create")
    cv.add_argument("--arch", default=None, help="target architecture, e.g. amd64 (default: host)")
    args = parser.parse_args(argv)

    try:
        convert(args.src, args.dst, args.arch)
    except (ImageError, OSError, tarfile.TarError, KeyError) as e:
        print(f"oci: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
