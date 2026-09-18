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
    """List world-readable files below root, the ones dnsmasq serves"""
    cmd = ("find", root, "-type", "f", "-perm", "-004",
           "-exec", "stat", "-c", "%s %Y %n", "{}", "+")
    files = []

    for line in HOST.run_multiline(cmd, []):
        size, mtime, path = line.split(" ", 2)
        files.append({
            "name": path[len(root) + 1:],
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
