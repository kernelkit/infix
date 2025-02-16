from . import common

from . import bridge
from . import ethernet
from . import ip
from . import lag
from . import tun
from . import veth
from . import vlan


def statistics(iplink):
    statistics = {}

    if rx := iplink.get("stats64", {}).get("rx"):
        if octets := rx.get("bytes"):
            statistics["in-octets"] = str(octets)

    if tx := iplink.get("stats64", {}).get("tx"):
        if octets := tx.get("bytes"):
            statistics["out-octets"] = str(octets)

    return statistics


def iplink2yang_type(iplink):
    match iplink["link_type"]:
        case "loopback":
            return "infix-if-type:loopback"
        case "gre"|"gre6":
            return "infix-if-type:gre"
        case "ether":
            pass
        case _:
            return "infix-if-type:other"

    match iplink.get("linkinfo", {}).get("info_kind"):
        case "bond":
            return "infix-if-type:lag"
        case "bridge":
            return "infix-if-type:bridge"
        case "dummy":
            return "infix-if-type:dummy"
        case "gretap"|"ip6gretap":
            return "infix-if-type:gretap"
        case "vxlan":
            return "infix-if-type:vxlan"
        case "veth":
            return "infix-if-type:veth"
        case "vlan":
            return "infix-if-type:vlan"

    return "infix-if-type:ethernet"


def iplink2yang_lower(iplink):
    if not (kind := iplink.get("linkinfo",{}).get("info_slave_kind")):
        return None

    match kind:
        case "bridge":
            return "infix-interfaces:bridge-port"
        case "bond":
            return "infix-interfaces:lag-port"

    return None


def iplink2yang_operstate(iplink):
    xlate = {
        "DOWN":                "down",
        "UP":                  "up",
        "DORMANT":             "dormant",
        "TESTING":             "testing",
        "LOWERLAYERDOWN":      "lower-layer-down",
        "NOTPRESENT":          "not-present"
    }
    return xlate.get(iplink["operstate"], "unknown")


def interface_common(iplink, ipaddr):
    interface = {
        "type": iplink2yang_type(iplink),
        "name": iplink.get("ifname"),
        "if-index": iplink.get("ifindex"),

        "admin-status": "up" if "UP" in iplink.get("flags", "") else "down",
        "oper-status": iplink2yang_operstate(iplink),
    }

    if "ifalias" in iplink:
        interface["description"] = iplink["ifalias"]

    if "address" in iplink and not "POINTOPOINT" in iplink["flags"]:
        interface["phys-address"] = iplink["address"]

    if stats := statistics(iplink):
        interface["statistics"] = stats

    if ipv4 := ip.ipv4(ipaddr):
        interface["ietf-ip:ipv4"] = ipv4

    if ipv6 := ip.ipv6(ipaddr):
        interface["ietf-ip:ipv6"] = ipv6

    return interface


def interface(iplink, ipaddr):
    interface = interface_common(iplink, ipaddr)

    match interface["type"]:
        case "infix-if-type:bridge":
            if br := bridge.bridge(iplink):
                interface["infix-interfaces:bridge"] = br
            if brport := bridge.lower(iplink):
                interface["infix-interfaces:bridge-port"] = brport
        case "infix-if-type:lag":
            if lg := lag.lag(iplink):
                interface["infix-interfaces:lag"] = lg
        case "infix-if-type:ethernet":
            if eth := ethernet.ethernet(iplink):
                interface["ieee802-ethernet-interface:ethernet"] = eth
        case "infix-if-type:vxlan":
            if vxlan := tun.vxlan(iplink):
                interface["infix-interfaces:vxlan"] = vxlan
        case "infix-if-type:gre" | "infix-if-type:gretap":
            if gre := tun.gre(iplink):
                interface["infix-interfaces:gre"] = gre
        case "infix-if-type:veth":
            if ve := veth.veth(iplink):
                interface["infix-interfaces:veth"] = ve
        case "infix-if-type:vlan":
            if v := vlan.vlan(iplink):
                interface["infix-interfaces:vlan"] = v

    match iplink2yang_lower(iplink):
        case "infix-interfaces:bridge-port":
            if brport := bridge.lower(iplink):
                interface["infix-interfaces:bridge-port"] = brport
        case "infix-interfaces:lag-port":
            if lagport := lag.lower(iplink):
                interface["infix-interfaces:lag-port"] = lagport

    return interface


def interfaces(ifname=None):
    links = common.iplinks(ifname)
    addrs = common.ipaddrs(ifname)

    interfaces = []
    for ifname, iplink in links.items():
        if iplink.get("group") == "internal":
            continue

        ipaddr = addrs.get(ifname, {})

        interfaces.append(interface(iplink, ipaddr))

    return interfaces
