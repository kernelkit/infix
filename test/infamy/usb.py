def _get_hardware(target):
    xpath="/ietf-hardware:hardware"
    hw = target.get_data(xpath)
    return {} if hw is None else hw.get("hardware", {})

def get_usb_ports(target):
    hardware=_get_hardware(target)
    ports=[]
    for component in hardware.get("component"):
        if component.get("class") == "infix-hardware:usb":
            ports.append(component["name"])

    return ports

def get_usb_state(target, name):
    hardware=_get_hardware(target)
    for component in hardware.get("component"):
        if component.get("name") == name and component.get("class") == "infix-hardware:usb":
            return component["state"]["admin-state"]

def get_usb_devices(target):
    hardware=_get_hardware(target)
    devices=[]
    for component in hardware["component"]:
        if component.get("class") != "infix-hardware:usb-device":
            continue
        usb_device=component.get("usb-device", {})
        devices.append({"name": component["name"],
                        "parent": component.get("parent"),
                        "authorized": usb_device.get("authorized"),
                        "classes": [i.get("class") for i in usb_device.get("interface", [])]})

    return devices

def get_usb_device(target, name):
    for device in get_usb_devices(target):
        if device["name"] == name:
            return device
    return {}
