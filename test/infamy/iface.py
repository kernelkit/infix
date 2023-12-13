"""
Fetch interface status from remote device.
"""

def _iface_xpath(iface, path=None):
    """Compose complete XPath to a YANG node in /ietf-interfaces"""
    xpath = f"/ietf-interfaces:interfaces/interface[name='{iface}']"
    if path:
        xpath.join(f"/{path}")
    return xpath

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
    try:
        content = target.get_data(_iface_xpath(iface, param))
        return _iface_extract_param(content, param)
    except:
        return None

def interface_exist(target, iface):
    """Verify that the target interface exists"""
    return _iface_get_param(target, iface, "name") is not None

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
