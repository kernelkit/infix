"""DHCP for bearers where ModemManager reports IP method 'dhcp'.

Modems with an Ethernet-style data port (ECM/NCM) get their address by
DHCP on that port.  modemd runs udhcpc on it, with this module's main()
as the udhcpc script.  The script only records the lease, modemd applies
it like an address handed out by ModemManager.
"""
import ipaddress
import os
import subprocess
import sys

from . import freadj, fwritej_atomic, rmf, stopproc

LEASEDIR = "/run/modemd/dhcp"
SCRIPT = "/usr/libexec/modemd/modem-dhcp"


def _path(ifname):
    return "%s/%s.json" % (LEASEDIR, ifname)


def start(ifname):
    """Start udhcpc on ifname, returns the process."""
    rmf(_path(ifname))
    return subprocess.Popen(["udhcpc", "-f", "-S", "-t", "3", "-T", "5",
                             "-A", "10", "-i", ifname, "-s", SCRIPT])


def stop(proc, ifname):
    """Stop the udhcpc process for ifname and forget its lease."""
    stopproc(proc)
    rmf(_path(ifname))


def lease(ifname):
    """Current lease for ifname, in ModemManager ipv4-config terms, or None."""
    return freadj(_path(ifname))


def main():
    """udhcpc script: record the lease on bound/renew, drop it otherwise."""
    action = sys.argv[1] if len(sys.argv) > 1 else ""
    ifname = os.environ.get("interface", "")
    if not ifname:
        sys.exit(1)

    if action not in ("bound", "renew"):
        rmf(_path(ifname))
        return

    mask = os.environ.get("subnet", "255.255.255.255")
    routers = os.environ.get("router", "").split()
    os.makedirs(LEASEDIR, exist_ok=True)
    fwritej_atomic(_path(ifname), {
        "address": os.environ["ip"],
        "prefix": str(ipaddress.IPv4Network("0.0.0.0/%s" % mask).prefixlen),
        "gateway": routers[0] if routers else "--",
        "dns": os.environ.get("dns", "").split(),
    })
