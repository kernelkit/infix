"""
Collect operational data for infix-services.yang
"""
from datetime import datetime, timezone

from .host import HOST

TFTP_CONF = "/etc/dnsmasq.d/tftp.conf"


def tftp_root():
    """TFTP root from the dnsmasq snippet written by confd, None if disabled"""
    for line in HOST.read_multiline(TFTP_CONF, []):
        if line.startswith("tftp-root="):
            return line[len("tftp-root="):]

    return None


def tftp_files(root):
    """List world-readable files below root, the ones dnsmasq serves

    Symlinks are followed, like the server does, and a name with a
    newline in it is skipped.
    """
    cmd = ("find", "-L", root, "-type", "f", "-perm", "-004",
           "-exec", "stat", "-L", "-c", "%s %Y %n", "{}", "+")
    prefix = root.rstrip("/") + "/"
    files = []

    for line in HOST.run_multiline(cmd, []):
        fields = line.split(" ", 2)
        if len(fields) != 3 or not fields[2].startswith(prefix):
            continue

        size, mtime, path = fields
        files.append({
            "name": path[len(prefix):],
            "size": size,
            "modified": datetime.fromtimestamp(int(mtime), timezone.utc).isoformat(),
        })

    return sorted(files, key=lambda f: f["name"])


def operational():
    """Return operational status for infix-services"""
    root = tftp_root()
    if not root:
        return {}

    return {
        "infix-services:tftp": {
            "files": {
                "file": tftp_files(root)
            }
        }
    }
