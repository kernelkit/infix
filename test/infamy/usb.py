def _get_hardware(target):
    xpath="/ietf-hardware:hardware"
    return target.get_data(xpath)["hardware"]

def get_usb_ports(target):
    hardware=_get_hardware(target)
    ports=[]
    for component in hardware["component"]:
        if component.get("class") == "infix-hardware:usb":
            ports.append(component["name"])

    return ports

def get_usb_state(target, name):
    hardware=_get_hardware(target)
    for component in hardware["component"]:
        if component.get("name") == name and component.get("class") == "infix-hardware:usb":
            return component["state"]["admin-state"]
