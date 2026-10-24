"""
Fetch interface status from remote device.
"""
import ipaddress
from datetime import datetime


def get_xpath(iface, path=None):
    """Compose complete XPath to a YANG node in /ietf-interfaces"""
    xpath = f"/ietf-interfaces:interfaces/interface[name='{iface}']"
    if path is not None:
        xpath = f"{xpath}/{path}"
    return xpath


def _extract_param(json_content, param):
    """Returns (extracted) value for parameter 'param'"""
    interfaces = json_content.get('interfaces')
    if not interfaces:
        return None

    for interface in interfaces.get('interface'):
        if param not in interface:
            continue
        return interface[param]

    return None


def get_param(target, iface, param=None):
    """Fetch target dict for iface and extract param from JSON"""
    content = target.get_data(get_xpath(iface, param))
    if content is None:
        return None
    return _extract_param(content, param)


def exist(target, iface):
    """Verify that the target interface exists"""
    return get_param(target, iface, "name") is not None


def address_exist(target, iface, address, prefix_length=None, proto="dhcp"):
    """Check if 'address' is set on iface"""
    if not prefix_length:
        if ':' in address:
            prefix_length = 64
        else:
            prefix_length = 24

    addrs = get_ipv4_address(target, iface)
    if addrs:
        for addr in addrs:
            if addr['origin'] == proto and addr['ip'] == address and\
               addr['prefix-length'] == prefix_length:
                return True

    addrs = get_ipv6_address(target, iface)
    if addrs:
        for addr in addrs:
            if addr['origin'] == proto and addr['ip'] == address and\
               addr['prefix-length'] == prefix_length:
                return True

    return False


def get_ipv4_address(target, iface):
    """Fetch interface IPv4 addresses from operational"""
    # The interface array is different in restconf/netconf, netconf has
    # a keyed list but restconf has a numbered list, i think i read that
    # this was a bug in rousette, but have not found it.
    interface = target.get_iface(iface)
    if interface is None:
        raise "Interface not found"

    ip = interface.get("ipv4") or interface.get("ietf-ip:ipv4")
    if ip is None or 'address' not in ip:
        return None
    return ip['address']


def get_ipv6_address(target, iface):
    """Fetch interface IPv6 addresses from operational"""
    # The interface array is different in restconf/netconf, netconf has
    # a keyed list but restconf has a numbered list, i think i read that
    # this was a bug in rousette, but have not found it.
    interface = target.get_iface(iface)
    if interface is None:
        raise "Interface not found"

    ip = interface.get("ipv6") or interface.get("ietf-ip:ipv6")
    if ip is None or 'address' not in ip:
        return None
    return ip['address']


def get_phys_address(target, iface):
    """Fetch interface MAC address (operational status)"""
    return get_param(target, iface, "phys-address")


def get_oper_status(target, iface):
    """Get interface operational status (up/down/etc)"""
    return get_param(target, iface, "oper-status")


def get_last_change(target, iface):
    """Get when the interface entered its operational status, None if unknown"""
    stamp = get_param(target, iface, "last-change")
    return datetime.fromisoformat(stamp) if stamp else None


def is_oper_up(target, iface):
    """Check if interface operational status is 'up'"""
    return get_oper_status(target, iface) == "up"


def _get_neighbors(target, iface, proto):
    interface = target.get_iface(iface)
    if interface is None:
        return None
    ip = interface.get(proto) or interface.get(f"ietf-ip:{proto}")
    return ip.get("neighbor") if ip else None


def neighbor_exist(target, iface, address, lladdr=None, origin=None):
    """Check if neighbor 'address' exists on iface, optionally matching lladdr and origin"""
    for proto in ("ipv4", "ipv6"):
        for n in _get_neighbors(target, iface, proto) or []:
            if n.get("ip") != address:
                continue
            if lladdr and n.get("link-layer-address") != lladdr:
                continue
            if origin and n.get("origin") != origin:
                continue
            return True
    return False


def get_bridge(target, bridge):
    """Operational bridge container of bridge, empty if not (yet) there"""
    interface = target.get_iface(bridge) or {}
    return interface.get("bridge") or interface.get("infix-interfaces:bridge") or {}


def bridge_fdb(target, bridge):
    """Entries the get-forwarding-database action lists for bridge.

    vid and age are integers and offloaded a boolean on both transports.
    """
    xpath = f"/ietf-interfaces:interfaces/interface[name='{bridge}']" \
            "/infix-interfaces:bridge/get-forwarding-database"
    entries = target.call_action_output(xpath).get("entry", [])
    for entry in entries:
        for key in ("vid", "age"):
            if key in entry:
                entry[key] = int(entry[key])
        entry["offloaded"] = str(entry.get("offloaded")).lower() == "true"
    return entries


def bridge_fdb_entry(target, bridge, vid, address):
    """FDB entry for address in VLAN vid of bridge, None if missing"""
    for entry in bridge_fdb(target, bridge):
        if entry.get("vid") == vid and entry["address"] == address:
            return entry
    return None


def _get_bridge_mcast(target, bridge, vid=None):
    """Fetch the multicast settings of a bridge, or of one of its VLANs"""
    brif = get_bridge(target, bridge)
    if vid is None:
        return brif

    for vlan in brif.get("vlans", {}).get("vlan", []):
        if vlan.get("vid") == vid:
            return vlan

    return {}


def exist_bridge_multicast_filter(target, group, iface, bridge, vid=None):
    """Check if a bridge, or bridge VLAN, has a multicast filter for group with iface"""
    mcast = _get_bridge_mcast(target, bridge, vid)
    for f in mcast.get("multicast-filters", {}).get("multicast-filter", []):
        if f.get("group") == group:
            for p in f.get("ports"):
                if p["port"] == iface:
                    return True

    return False


def bridge_vlan_members(target, bridge, vid, tagging="untagged"):
    """Ports listed as tagged/untagged members of vid on bridge, from operational."""
    br = get_bridge(target, bridge)
    for vlan in (br.get("vlans") or {}).get("vlan") or []:
        if vlan.get("vid") == vid:
            return set(vlan.get(tagging) or [])
    return set()


def get_bridge_queriers(target, bridge, vid=None):
    """Fetch the active IGMP/MLD queriers of a bridge, or bridge VLAN, keyed by family"""
    mcast = _get_bridge_mcast(target, bridge, vid)
    queriers = mcast.get("multicast", {}).get("active-querier", [])
    return {q["family"]: q for q in queriers}


def get_bridge_router_ports(target, bridge, vid=None):
    """Fetch the multicast router ports of a bridge, or bridge VLAN"""
    mcast = _get_bridge_mcast(target, bridge, vid)
    return set(mcast.get("multicast", {}).get("router-ports", []))


def get_bridge_querier_election(targets, family, bridge="br0", vid=None):
    """Return (index of the local querier, its address) if all targets agree"""
    from .util import parallel  # circular: util -> netconf -> transport -> iface

    queriers = parallel(*(lambda t=t: get_bridge_queriers(t, bridge, vid).get(family)
                          for t in targets))
    if not all(queriers):
        return None

    addresses = {ipaddress.ip_address(q["address"]) for q in queriers}
    if len(addresses) != 1:
        return None

    local = [i for i, q in enumerate(queriers) if q["local"]]
    if len(local) != 1:
        return None

    return local[0], addresses.pop()
