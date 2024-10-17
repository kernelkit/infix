"""
Fetch interface status from remote device.
"""

def get_xpath(iface, path=None):
    """Compose complete XPath to a YANG node in /ietf-interfaces"""
    xpath=f"/ietf-interfaces:interfaces/interface[name='{iface}']"
    if not path is None:
        xpath=f"{xpath}/{path}"
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

def address_exist(target, iface, address, prefix_length = 24, proto="dhcp"):
    """Check if 'address' is set on iface"""
    addrs = get_ipv4_address(target, iface)
    if not addrs:
        return False
    for addr in addrs:
        if addr['origin'] == proto and addr['ip'] == address and addr['prefix-length'] == prefix_length:
            return True

def get_ipv4_address(target, iface):
    """Fetch interface IPv4 addresses from (operational status)"""
    # The interface array is different in restconf/netconf, netconf has a keyed list but
    # restconf has a numbered list, i think i read that this was a bug in rousette, but
    # have not found it.
    interface=target.get_iface(iface)
    if interface is None:
        raise "Interface not found"

    ipv4 = interface.get("ipv4") or interface.get("ietf-ip:ipv4")
    if ipv4 is None or 'address' not in ipv4:
        return None
    return ipv4['address']

def get_phys_address(target, iface):
    """Fetch interface MAC address (operational status)"""
    return get_param(target, iface, "phys-address")

def exist_bridge_multicast_filter(target, group, iface, bridge):
    # The interface array is different in restconf/netconf, netconf has a keyed list but
    # restconf has a numbered list, i think i read that this was a bug in rousette, but
    # have not found it.
    interface=target.get_iface(bridge)
    if interface is None:
        raise "Interface not found"

    brif = interface.get("bridge") or interface.get("infix-interfaces:bridge")
    if brif is None:
        return False

    for filter in brif.get("multicast-filters", {}).get("multicast-filter", {}):
        if filter.get("group") == group:
            for p in filter.get("ports"):
                if p["port"] == iface:
                    return True

    return False
