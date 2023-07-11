def extract_iface_param(json_content, param):
    """Returns (extracted) value for parameter 'param'"""
    interfaces = json_content.get('interfaces')
    if not interfaces:
        return None
    
    for interface in interfaces.get('interface'):
        if param not in interface:
            continue
        return interface[param]
    
    return None 

def interface_exist(target, iface):
    """Verify that the target interface exists"""
    try: 
        content = target.get_dict(f"/ietf-interfaces:interfaces/ietf-interfaces:interface[name='{iface}']")
        name = extract_iface_param(content, 'name')
        return (name != None)
    except:
        return False

def get_if_index(target, iface):
    """Return value of 'if-index' (interface index) parameter for the target interface"""
    try:
        content = target.get_dict(f"/ietf-interfaces:interfaces/ietf-interfaces:interface[name='{iface}']/ietf-interfaces:if-index")
        if_index = extract_iface_param(content, 'if-index')
        return if_index
    except:
        return None

def get_oper_status(target, iface):
    """Return value of 'oper-status' (operational status) parameter for the target interface"""
    try:
        content = target.get_dict(f"/ietf-interfaces:interfaces/ietf-interfaces:interface[name='{iface}']/ietf-interfaces:oper-status")
        oper_status = extract_iface_param(content, 'oper-status')
        return oper_status
    except:
        return None
    
def print_iface_status(target):
    """Print status parameters for all target interfaces"""
    try:
        content = target.get_dict(f"/ietf-interfaces:interfaces")
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