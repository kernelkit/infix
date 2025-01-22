from ..host import HOST

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

    mtu = ipaddr.get("mtu")
    if mtu and ipaddr.get("ifname") != "lo":
        ipv4["mtu"] = mtu

    if addrs := addresses(ipaddr, "inet"):
        ipv4["address"] = addrs

    return ipv4

def ipv6(ipaddr):
    ipv6 = {}

    if mtu := HOST.read(f"/proc/sys/net/ipv6/conf/{ipaddr['ifname']}/mtu"):
        ipv6["mtu"] = int(mtu.strip())

    if addrs := addresses(ipaddr, "inet6"):
        ipv6["address"] = addrs

    return ipv6
