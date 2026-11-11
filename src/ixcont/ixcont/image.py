"""Resolve a YANG image reference to a flattened image directory.

Supported references:

  oci:/path, oci-archive:/path, docker-archive:/path, dir:/path, /path
      A flattened image directory (rootfs + config.json), an OCI layout
      directory, or an OCI archive.  A bare name is looked up in /lib/oci
      and /var/lib/containers/images, with and without .tar/.tar.gz.
  http://, https://, ftp://
      An OCI archive to fetch and flatten, optionally checksum verified.
  docker://registry/name[:tag|@digest], or a bare registry/name
      Pulled with skopeo into an OCI layout, then flattened.  A bare name
      without a registry, e.g. nginx:alpine, is looked up locally first
      and then on docker.io, see /etc/containers/registries.conf.

Embedded images are flattened at build time and used in place.  Anything
else is flattened once into /var/lib/containers/images/<name>-<sha12>
and reused until the source archive changes, or until the container is
upgraded for a fetched image.  A fetched image lists the references it
was fetched as in <image>/refs, which is how it is found again.
"""
import hashlib
import os
import shutil
import subprocess
import tempfile

from . import oci, paths
from .state import ContainerError, log, sh

LOCAL = ("oci:", "oci-archive:", "docker-archive:", "dir:")
REMOTE = ("http://", "https://", "ftp://")
REGISTRY = "docker://"
ARCHIVE_EXT = (".tar.gz", ".tgz", ".tar")


class Image:
    def __init__(self, path, config, digest):
        self.path = path
        self.rootfs = os.path.join(path, "rootfs")
        self.config = config.get("config", {})
        self.id = digest


def _strip_ext(name):
    for ext in ARCHIVE_EXT:
        if name.endswith(ext):
            return name[:-len(ext)]
    return name


def _sha256(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def _candidates(path):
    base = os.path.basename(path)
    yield path
    yield _strip_ext(path)
    for d in (paths.BUILTIN, paths.IMAGES):
        yield os.path.join(d, base)
        yield os.path.join(d, _strip_ext(base))


def _flat(path):
    return Image(path, *oci.load_flat(path))


def _flatten(src, key):
    """Flatten src into the image cache under key, reuse when unchanged."""
    dst = os.path.join(paths.IMAGES, key)
    if oci.is_flat(dst):
        return _flat(dst)
    log(f"Flattening image {src} to {dst}")
    os.makedirs(paths.IMAGES, exist_ok=True)
    config, digest = oci.flatten(src, dst)
    return Image(dst, config, digest)


def _local(path):
    for cand in _candidates(path):
        if oci.is_flat(cand):
            return _flat(cand)
    for cand in _candidates(path):
        if os.path.isdir(cand) and os.path.isfile(os.path.join(cand, "index.json")):
            key = os.path.basename(cand.rstrip("/"))
            return _flatten(cand, key)
        if os.path.isfile(cand):
            key = f"{_strip_ext(os.path.basename(cand))}-{_sha256(cand)[:12]}"
            return _flatten(cand, key)
    raise ContainerError(f"cannot find image {path}")


def _verify(path, checksum):
    for algo, expect in (checksum or {}).items():
        h = hashlib.new(algo)
        with open(path, "rb") as f:
            for chunk in iter(lambda: f.read(1 << 20), b""):
                h.update(chunk)
        if h.hexdigest().lower() != expect.lower():
            os.unlink(path)
            raise ContainerError(f"{path}: {algo} checksum mismatch")
        log(f"{path} {algo} checksum verified OK")


def _fetch(url, checksum, creds=None):
    os.makedirs(paths.IMAGES, exist_ok=True)
    fd, tmp = tempfile.mkstemp(prefix=".dl-", dir=paths.IMAGES)
    os.close(fd)
    log(f"Fetching {url}")
    try:
        if url.startswith("ftp://"):
            sh("wget", "-q", "-O", tmp, url)
        else:
            cmd = ["curl", "-sSL", "--fail", "-o", tmp]
            if creds:
                cmd += ["-u", creds]
            sh(*cmd, url)
        _verify(tmp, checksum)
        key = f"{_strip_ext(os.path.basename(url))}-{_sha256(tmp)[:12]}"
        return _tag(_flatten(tmp, key), url)
    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)


def _pull(ref, creds=None):
    """Pull a registry image into the cache, keyed on its config digest."""
    name = ref[len(REGISTRY):]
    os.makedirs(paths.IMAGES, exist_ok=True)
    tmp = tempfile.mkdtemp(prefix=".pull-", dir=paths.IMAGES)
    log(f"Pulling {ref}")
    try:
        cmd = ["skopeo", "copy"]
        if creds:
            cmd += ["--src-creds", creds]
        sh(*cmd, ref, "oci:" + tmp)
        config, digest = oci.load_layout(tmp)
    except Exception:
        shutil.rmtree(tmp, ignore_errors=True)
        raise
    key = f"{_pull_prefix(name)}{digest[:12]}"
    try:
        return _tag(_flatten(tmp, key), ref)
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def _pull_prefix(name):
    """Cache name of a registry image: last path element and tag."""
    repo, _, _ = name.partition("@")
    base = repo.rsplit("/", 1)[-1]
    return base.replace(":", "-") + "-"


def _refs(path):
    try:
        with open(os.path.join(path, "refs")) as f:
            return f.read().split()
    except OSError:
        return []


def _tag(img, ref):
    """Record that ref now is img, and no longer any other cached image."""
    for d in cached():
        path = os.path.join(paths.IMAGES, d)
        old = _refs(path)
        new = [r for r in old if r != ref] + ([ref] if path == img.path else [])
        if new != old:
            with open(os.path.join(path, "refs"), "w") as f:
                f.write("".join(r + "\n" for r in new))
    return img


def _cached_ref(ref):
    for d in cached():
        path = os.path.join(paths.IMAGES, d)
        if ref in _refs(path):
            return path
    return None


def _source(ref):
    """What to fetch for ref, a URL or docker:// reference, None when it
    can only be a local image."""
    if ref.startswith(REMOTE) or ref.startswith(REGISTRY):
        return ref
    if ref.startswith(LOCAL) or ref.startswith("/"):
        return None
    return REGISTRY + ref


def _path(ref):
    for prefix in LOCAL:
        if ref.startswith(prefix):
            return ref[len(prefix):]
    return ref


def _wait_for_network():
    """Block until an address or route changes, at most a minute."""
    subprocess.run(["timeout", "60", "sh", "-c", "ip monitor address route | head -n1"],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)


def resolve(ref, checksum=None, force=False, creds=None):
    """Return the Image for a YANG image reference.

    force re-fetches a remote image, it has no effect on local ones since
    the cache key is the archive's own checksum.  A remote image that
    cannot be fetched is retried on every network change, the service
    stays in its setup phase until then.
    """
    src = _source(ref)
    if src is None:
        return _local(_path(ref))
    if src != ref:
        # A bare name may also be a loaded image
        try:
            return _local(ref)
        except ContainerError:
            pass

    # What a reference leads to is only known by fetching it, so a
    # fetched image is reused until the container is upgraded.
    if not force:
        path = _cached_ref(src)
        if path:
            return _flat(path)
    while True:
        try:
            if src.startswith(REGISTRY):
                return _pull(src, creds)
            return _fetch(src, checksum, creds)
        except ContainerError as e:
            log(f"{e}, waiting for network changes ...")
            _wait_for_network()


def cached_path(ref):
    """The unpacked directory an image reference resolves to now, or None.

    Never fetches or unpacks anything, for callers that only need to know
    what is already on disk, like the boot reconcile.
    """
    src = _source(ref)
    if src != ref:
        for cand in _candidates(_path(ref)):
            if oci.is_flat(cand):
                return cand
    return _cached_ref(src) if src else None


def load(src, name=None, creds=None):
    """The oci-load RPC: flatten a local archive, URL or registry image."""
    if src.startswith(REMOTE):
        return _fetch(src, None, creds)
    if src.startswith(REGISTRY):
        return _pull(src, creds)
    key = name.replace(":", "-") if name else _strip_ext(os.path.basename(src))
    for cand in _candidates(src):
        if os.path.exists(cand):
            os.makedirs(paths.IMAGES, exist_ok=True)
            config, digest = oci.flatten(cand, os.path.join(paths.IMAGES, key))
            return Image(os.path.join(paths.IMAGES, key), config, digest)
    raise ContainerError(f"cannot find {src}")


def cached():
    """Flattened images in /var/lib/containers/images."""
    if not os.path.isdir(paths.IMAGES):
        return []
    return sorted(d for d in os.listdir(paths.IMAGES) if oci.is_flat(os.path.join(paths.IMAGES, d)))


def builtin():
    if not os.path.isdir(paths.BUILTIN):
        return []
    return sorted(d for d in os.listdir(paths.BUILTIN) if oci.is_flat(os.path.join(paths.BUILTIN, d)))


def prune(keep):
    """Remove cached images whose path is not in keep."""
    removed = []
    for d in cached():
        path = os.path.join(paths.IMAGES, d)
        if path not in keep:
            shutil.rmtree(path, ignore_errors=True)
            removed.append(path)
    return removed
