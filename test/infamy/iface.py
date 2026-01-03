"""
Fetch interface status from remote device.
"""


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


def is_oper_up(target, iface):
    """Check if interface operational status is 'up'"""
    return get_oper_status(target, iface) == "up"


def exist_bridge_multicast_filter(target, group, iface, bridge):
    """Check if a bridge has a multicast filter for group with iface"""
    # The interface array is different in restconf/netconf, netconf has
    # a keyed list but restconf has a numbered list, i think i read that
    # this was a bug in rousette, but have not found it.
    interface = target.get_iface(bridge)
    if interface is None:
        raise "Interface not found"

    brif = interface.get("bridge") or interface.get("infix-interfaces:bridge")
    if brif is None:
        return False

    for f in brif.get("multicast-filters", {}).get("multicast-filter", {}):
        if f.get("group") == group:
            for p in f.get("ports"):
                if p["port"] == iface:
                    return True

    return False
