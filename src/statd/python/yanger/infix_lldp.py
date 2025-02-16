from .host import HOST
from collections import defaultdict

def operational():
    """Retrieve LLDP neighbor information and store in remote-systems-data under the correct port."""

    # Reference: https://www.ieee802.org/1/files/public/YANGs/ieee802-types.yang
    subtype_mapping = {
        "component": "chassis-component",
        "ifalias": "interface-alias",
        "port": "port-component",
        "mac": "mac-address",
        "ip": "network-address",
        "ifname": "interface-name",
        "local": "local"
    }

    DEFAULT_MAC = "00-00-00-00-00-00"

    port_data = defaultdict(lambda: {"remote-systems-data": [], "dest-mac-address": None})

    data = HOST.run_json(["lldpcli", "show", "neighbors", "-f", "json"])

    interfaces = data.get("lldp", {}).get("interface", [])
    
    if isinstance(interfaces, dict):
        interfaces = [interfaces]

    for iface_entry in interfaces:
        for iface_name, iface_data in iface_entry.items():
            remote_index = int(iface_data.get("rid", 0))
            time_mark = parse_time(iface_data.get("age"))

            chassis = iface_data.get("chassis", {})
            chassis_id_type, chassis_id_value = extract_chassis_id(chassis, subtype_mapping)

            port_info = iface_data.get("port", {})
            port_id_type = subtype_mapping.get(port_info.get("id", {}).get("type"), "unknown")
            port_id_value = port_info.get("id", {}).get("value", "")

            dest_mac_address = (
                chassis_id_value.replace(":", "-") if chassis_id_type == "mac-address" else
                port_id_value.replace(":", "-") if port_id_type == "mac-address" else
                DEFAULT_MAC
            )

            remote_entry = {
                "time-mark": time_mark,
                "remote-index": remote_index,
                "chassis-id-subtype": chassis_id_type,
                "chassis-id": chassis_id_value,
                "port-id-subtype": port_id_type,
                "port-id": port_id_value
            }

            port_data[iface_name]["remote-systems-data"].append(remote_entry)

            if port_data[iface_name]["dest-mac-address"] is None:
                port_data[iface_name]["dest-mac-address"] = dest_mac_address

    formatted_output = {
        "ieee802-dot1ab-lldp:lldp": {
            "port": [
                {
                    "name": port_name,
                    "dest-mac-address": port_info["dest-mac-address"],
                    "remote-systems-data": port_info["remote-systems-data"]
                }
                for port_name, port_info in port_data.items() if port_info["remote-systems-data"]
            ]
        }
    }

    return formatted_output

def extract_chassis_id(chassis_block, subtype_mapping):
    if "id" in chassis_block:
        id_info = chassis_block["id"]
        return subtype_mapping.get(id_info.get("type"), "unknown"), id_info.get("value", "")

    for _, value in chassis_block.items():
        if isinstance(value, dict) and "id" in value:
            id_info = value["id"]
            return subtype_mapping.get(id_info.get("type"), "unknown"), id_info.get("value", "")

    return "unknown", ""

def parse_time(time_str):
    """Convert LLDP time format to seconds"""
    import re
    if time_str:
        match = re.search(r"(\d+)\s*day[s]*,\s*(\d+):(\d+):(\d+)", time_str)
        if match:
            days, hours, minutes, seconds = map(int, match.groups())
            return days * 86400 + hours * 3600 + minutes * 60 + seconds
    return 0
