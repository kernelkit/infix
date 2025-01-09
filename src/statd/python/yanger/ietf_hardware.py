import datetime
import os

from .common import insert
from .host import HOST


def vpd_vendor_extensions(data):
    vendor_extensions = []
    for ext in data:
        vendor_extension = {}
        vendor_extension["iana-enterprise-number"] = ext[0]
        vendor_extension["extension-data"] = ext[1]
        vendor_extensions.append(vendor_extension)
    return vendor_extensions


def vpd_component(vpd):
    component = {}
    component["name"] = vpd.get("board")
    component["infix-hardware:vpd-data"] = {}

    if vpd.get("data"):
        component["class"] = "infix-hardware:vpd"
        if vpd["data"].get("manufacture-date"):
            mfgdate = datetime.datetime.strptime(vpd["data"]["manufacture-date"],
                                                 "%m/%d/%Y %H:%M:%S")
            component["mfg-date"] = mfgdate.strftime("%Y-%m-%dT%H:%M:%SZ")
        if vpd["data"].get("manufacter"):
            component["mfg-name"] = vpd["data"]["manufacturer"]
        if vpd["data"].get("product-name"):
            component["model-name"] = vpd["data"]["product-name"]
        if vpd["data"].get("serial-number"):
            component["serial-num"] = vpd["data"]["serial-number"]

        # Set VPD-data entrys
        for k, v in vpd["data"].items():
            if vpd["data"].get(k):
                if k != "vendor-extension":
                    component["infix-hardware:vpd-data"][k] = v
                else:
                    vendor_extensions=vpd_vendor_extensions(v)
                    component["infix-hardware:vpd-data"]["infix-hardware:vendor-extension"] = vendor_extensions
    return component


def vpd_components(systemjson):
    return [vpd_component(vpd) for vpd in systemjson.get("vpd", {}).values()]


def usb_port_components(systemjson):
    usb_ports = systemjson.get("usb-ports", [])

    ports=[]
    names=[]
    for usb_port in usb_ports:
        port={}
        if usb_port.get("path"):
            if usb_port["name"] in names:
                continue

            path = usb_port["path"]
            if os.path.basename(path) == "authorized_default":
                # TODO: Untestable. Should be done via the host API
                if os.path.exists(path):
                    with open(path, "r") as f:
                        names.append(usb_port["name"])
                        data = int(f.readline().strip())
                        enabled = "unlocked" if data == 1 else "locked"
                        port["state"] = {}
                        port["state"]["admin-state"] = enabled
                        port["name"] = usb_port["name"]
                        port["class"] = "infix-hardware:usb"
                        port["state"]["oper-state"] = "enabled"
                        ports.append(port)

    return ports


def operational():
    systemjson = HOST.read_json("/run/system.json")

    return {
        "ietf-hardware:hardware": {
            "component":
            vpd_components(systemjson) +
            usb_port_components(systemjson) +
            [],
        },
    }
