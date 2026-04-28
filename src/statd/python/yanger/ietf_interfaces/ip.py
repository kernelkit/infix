import ipaddress

from ..host import HOST
from . import common


def neigh_state(states):
    xlate = {
        "REACHABLE":  "reachable",
        "STALE":      "stale",
        "DELAY":      "delay",
        "PROBE":      "probe",
        "INCOMPLETE": "incomplete",
    }
    return next((xlate[s] for s in states if s in xlate), None)


def neighbors(ifname, family):
    result = []
    for entry in common.ipneighs().get(ifname, []):
        dst = entry.get("dst", "")
        try:
            version = ipaddress.ip_address(dst).version
        except ValueError:
            continue

        if version != (4 if family == "inet" else 6):
            continue

        lladdr = entry.get("lladdr")
        states = entry.get("state", [])

        if not lladdr:
            continue

        origin = "static" if "PERMANENT" in states else "dynamic"
        neigh = {
            "ip":                 dst,
            "link-layer-address": lladdr,
            "origin":             origin,
        }

        if family == "inet6":
            if state := neigh_state(states):
                neigh["state"] = state
            if entry.get("router"):
                neigh["is-router"] = [None]

        result.append(neigh)

    return result


def inet2yang_origin(inet):
    """Translate kernel IP address origin to YANG"""
    xlate = {
        "kernel_ll":        "link-layer",
        "kernel_ra":        "link-layer",
        "static":           "static",
        "dhcp":             "dhcp",
        "random":           "random",
    }
    proto = inet.get("protocol")

    if proto in ("kernel_ll", "kernel_ra"):
        if "stable-privacy" in inet:
            return "random"

    return xlate.get(proto, "other")


def addresses(ipaddr, proto):
    addrs = []
    for inet in ipaddr.get("addr_info", []):
        if inet.get("family") != proto:
            continue

        addrs.append({
            "ip": inet.get("local"),
            "prefix-length": inet.get("prefixlen"),
            "origin": inet2yang_origin(inet),
        })

    return addrs

def ipv4(ipaddr):
    ipv4 = {}
    ifname = ipaddr.get("ifname")

    mtu = ipaddr.get("mtu")
    if mtu and ifname != "lo":
        ipv4["mtu"] = mtu

    if addrs := addresses(ipaddr, "inet"):
        ipv4["address"] = addrs

    if neighs := neighbors(ifname, "inet"):
        ipv4["neighbor"] = neighs

    return ipv4

def ipv6(ipaddr):
    ipv6 = {}
    ifname = ipaddr.get("ifname")

    if mtu := HOST.read(f"/proc/sys/net/ipv6/conf/{ifname}/mtu"):
        ipv6["mtu"] = int(mtu.strip())

    if addrs := addresses(ipaddr, "inet6"):
        ipv6["address"] = addrs

    if neighs := neighbors(ifname, "inet6"):
        ipv6["neighbor"] = neighs

    return ipv6
