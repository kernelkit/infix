"""
Fetch interface status from remote device.
"""

def _iface_extract_param(json_content, param):
    """Returns (extracted) value for parameter 'param'"""
    interfaces = json_content.get('interfaces')
    if not interfaces:
        return None

    for interface in interfaces.get('interface'):
        if param not in interface:
            continue
        return interface[param]

    return None

def _iface_get_param(target, iface, param=None):
    """Fetch target dict for iface and extract param from JSON"""
    content = target.get_data(target.get_iface_xpath(iface, param))
    return _iface_extract_param(content, param)

def interface_exist(target, iface):
    """Verify that the target interface exists"""
    return _iface_get_param(target, iface, "name") is not None

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

def get_if_index(target, iface):
    """Fetch interface 'if-index' (operational status)"""
    return _iface_get_param(target, iface, "if-index")

def get_oper_status(target, iface):
    """Fetch interface 'oper-status' (operational status)"""
    return _iface_get_param(target, iface, "oper-status")

def get_phys_address(target, iface):
    """Fetch interface MAC address (operational status)"""
    return _iface_get_param(target, iface, "phys-address")

def get_oper_up(target,iface):
    state=get_oper_status(target,iface)
    return state == "up"

def print_iface(target, iface):
    data = target.get_data(_iface_xpath(iface, None))
    print(data)

def print_all(target):
    """Print status parameters for all target interfaces"""
    try:
        content = target.get_dict("/ietf-interfaces:interfaces")
        interfaces = content.get('interfaces')
        if interfaces:
            interface_list = interfaces.get('interface')
            if interface_list and isinstance(interface_list, list):
                col1 = "name"
                col2 = "if-index"
                col3 = "oper-status"
                print('-'*36)
                print(f"{col1: <12}{col2: <12}{col3: <12}")
                print('-'*36)
                for interface in interface_list:
                    print(f"{interface['name']: <12}"
                          f"{interface['if-index']: <12}"
                          f"{interface['oper-status']: <12}")
                print('-'*36)
    except:
        print(f"Failed to get interfaces' status from target {target}")

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
