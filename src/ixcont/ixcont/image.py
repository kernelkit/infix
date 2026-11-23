"""Resolve a YANG image reference to a SIF image.

A container only ever runs from a SIF image, see sif.py.  Supported
references:

  /path, oci:/path, oci-archive:/path
      A SIF image, used where it is, or an OCI layout directory or OCI
      archive, converted to one.  A bare name is looked up in /lib/oci
      and /var/lib/containers/images, with or without .sif.
  http://, https://, ftp://
      A SIF image or an OCI archive to fetch, optionally checksum
      verified.
  docker://registry/name[:tag|@digest], or a bare registry/name
      Pulled with skopeo and converted, as are the other transports of
      skopeo, docker-archive: and dir:.  A bare name without a registry,
      e.g. nginx:alpine, is looked up locally first and then on
      docker.io, see /etc/containers/registries.conf.

The images in the system image are used in place.  Anything fetched or
converted is one file in /var/lib/containers/images, named after its
reference, and reused until the container is upgraded.  A converted
image is not signed.
"""
import hashlib
import os
import shutil
import subprocess
import tarfile
import tempfile
import time

from . import oci, paths, sif
from .state import ContainerError, log, sh

LOCAL = ("oci:", "oci-archive:")
REMOTE = ("http://", "https://", "ftp://")
REGISTRY = "docker://"
SKOPEO = (REGISTRY, "docker-archive:", "dir:")
EXT = (".tar.gz", ".tgz", ".tar", ".sif")


class Image:
    def __init__(self, path):
        try:
            img = sif.Sif(path)
            self.config = img.json(sif.CONFIG).get("config", {})
            self.id = img.json(sif.SOURCE).get("digest") or img.id
        except (sif.SifError, OSError) as e:
            raise ContainerError(str(e))
        self.path = path
        self.offset = img.offset


def _strip_ext(name):
    for ext in EXT:
        if name.endswith(ext):
            return name[:-len(ext)]
    return name


def _stored(ref, tag=""):
    """Where the image of ref is kept, tag telling versions of it apart."""
    base = _strip_ext(os.path.basename(ref.rstrip("/"))).replace(":", "-")
    digest = hashlib.sha256((ref + tag).encode()).hexdigest()[:12]
    return os.path.join(paths.IMAGES, f"{base}-{digest}.sif")


def _convert(src, dst, source):
    log(f"Converting {source} to {dst}")
    try:
        oci.convert(src, dst, source=source)
    except (oci.ImageError, OSError, tarfile.TarError, KeyError, ValueError) as e:
        raise ContainerError(f"{source}: {e}")
    return Image(dst)


def _candidates(path):
    """A reference may leave out .sif, and name the archive an image in
    the system image once was."""
    for d in ("", paths.BUILTIN, paths.IMAGES):
        cand = path if not d else os.path.join(d, os.path.basename(path))
        yield cand
        yield _strip_ext(cand) + ".sif"


def _local(path, convert=True):
    for cand in _candidates(path):
        if sif.is_sif(cand):
            return Image(cand)

    # An OCI layout or archive is converted again when it has changed
    if os.path.exists(path):
        st = os.stat(path)
        dst = _stored(os.path.abspath(path), f"{st.st_size}-{st.st_mtime_ns}")
        if sif.is_sif(dst):
            return Image(dst)
        if convert:
            return _convert(path, dst, path)
    raise ContainerError(f"cannot find image {path}")


def _verify(path, checksum):
    for algo, expect in (checksum or {}).items():
        h = hashlib.new(algo)
        with open(path, "rb") as f:
            for chunk in iter(lambda: f.read(1 << 20), b""):
                h.update(chunk)
        if h.hexdigest().lower() != expect.lower():
            raise ContainerError(f"{path}: {algo} checksum mismatch")
        log(f"{path} {algo} checksum verified OK")


def _fetch(url, dst, checksum=None, creds=None):
    os.makedirs(paths.IMAGES, exist_ok=True)
    fd, tmp = tempfile.mkstemp(prefix=".fetch-", dir=paths.IMAGES)
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
        if not sif.is_sif(tmp):
            return _convert(tmp, dst, url)
        os.replace(tmp, dst)
        return Image(dst)
    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)


def _pull(ref, dst, creds=None):
    os.makedirs(paths.IMAGES, exist_ok=True)
    tmp = tempfile.mkdtemp(prefix=".pull-", dir=paths.IMAGES)
    log(f"Pulling {ref}")
    try:
        cmd = ["skopeo", "copy"]
        if creds:
            cmd += ["--src-creds", creds]
        sh(*cmd, ref, "oci:" + tmp)
        return _convert(tmp, dst, ref)
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def _source(ref):
    """What to fetch for ref, a URL or a skopeo reference, None when it
    can only be a local image."""
    if ref.startswith(REMOTE) or ref.startswith(SKOPEO):
        return ref
    if ref.startswith(LOCAL) or ref.startswith("/"):
        return None
    return REGISTRY + ref


def _path(ref):
    for prefix in LOCAL:
        if ref.startswith(prefix):
            return ref[len(prefix):]
    return ref


def _get(src, dst, checksum=None, creds=None):
    if src.startswith(REMOTE):
        return _fetch(src, dst, checksum, creds)
    return _pull(src, dst, creds)


def _wait_for_network():
    """Block until an address or route changes, at most a minute."""
    subprocess.run(["timeout", "60", "sh", "-c", "ip monitor address route | head -n1"],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)


def resolve(ref, checksum=None, force=False, creds=None):
    """Return the Image for a YANG image reference.

    force fetches a remote image again, it has no effect on local ones.
    A remote image that cannot be fetched is retried on every network
    change, the service stays in its setup phase until then.
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
    dst = _stored(src)
    if not force and sif.is_sif(dst):
        return Image(dst)
    while True:
        try:
            return _get(src, dst, checksum, creds)
        except ContainerError as e:
            log(f"{e}, waiting for network changes ...")
            _wait_for_network()


def cached_path(ref):
    """The image file a reference resolves to now, or None.

    Never fetches or converts anything, for callers that only need to
    know what is already on disk, like the boot reconcile.
    """
    src = _source(ref)
    if src != ref:
        try:
            return _local(_path(ref), convert=False).path
        except ContainerError:
            pass
    if src and sif.is_sif(_stored(src)):
        return _stored(src)
    return None


def load(src, name=None, creds=None):
    """Keep the image of a file, URL or skopeo reference, as name if given."""
    if name and name != os.path.basename(name):
        raise ContainerError(f"{name}: not a name for an image")
    dst = os.path.join(paths.IMAGES, name + ".sif") if name else None

    if _source(src) == src:
        return _get(src, dst or _stored(src), creds=creds)

    path = _path(src)
    if not os.path.exists(path):
        raise ContainerError(f"cannot find {src}")
    dst = dst or os.path.join(paths.IMAGES, _strip_ext(os.path.basename(path)) + ".sif")
    if not sif.is_sif(path):
        return _convert(path, dst, path)
    os.makedirs(paths.IMAGES, exist_ok=True)
    shutil.copyfile(path, dst + ".tmp")
    os.replace(dst + ".tmp", dst)
    return Image(dst)


def _images(directory):
    if not os.path.isdir(directory):
        return []
    return sorted(os.path.join(directory, f) for f in os.listdir(directory)
                  if sif.is_sif(os.path.join(directory, f)))


def stored():
    """The images in /var/lib/containers/images."""
    return _images(paths.IMAGES)


def builtin():
    """The images in the system image."""
    return _images(paths.BUILTIN)


def prune(referenced):
    """Remove the images in the store that referenced() does not list,
    return what went.  One that appears meanwhile is left alone."""
    candidates = stored()
    keep = referenced()
    removed = [path for path in candidates if path not in keep]
    for path in removed:
        os.unlink(path)
    return removed


def sweep():
    """Remove what an interrupted fetch or conversion left before this boot."""
    if not os.path.isdir(paths.IMAGES):
        return
    with open("/proc/uptime") as f:
        boot = time.time() - float(f.read().split()[0])
    for entry in os.listdir(paths.IMAGES):
        path = os.path.join(paths.IMAGES, entry)
        if sif.is_sif(path) or os.lstat(path).st_mtime > boot:
            continue
        if os.path.isdir(path) and not os.path.islink(path):
            shutil.rmtree(path, ignore_errors=True)
        else:
            os.unlink(path)
