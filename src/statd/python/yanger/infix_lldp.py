from .host import HOST
from collections import defaultdict

def operational():
    """Retrieve LLDP neighbor information and store in remote-systems-data under the correct port."""

    # https://www.ieee802.org/1/files/public/YANGs/ieee802-types.yang
    # <chassis-component> and <port-component> subtypes are not supported in
    # https://github.com/lldpd/lldpd until v1.0.19
    chassis_id_subtype_mapping = {
        #"unhandled": "chassis-component",
        "ifalias": "interface-alias",
        #"unhandled": "port-component",
        "mac": "mac-address",
        "ip": "network-address",
        "ifname": "interface-name",
        "local": "local"
    }

    # https://www.ieee802.org/1/files/public/YANGs/ieee802-types.yang
    # <port-component> and <agent-circuit-id> subtypes are not supported in
    # https://github.com/lldpd/lldpd until v1.0.19
    port_id_subtype_mapping = {
        "ifalias": "interface-alias",
        #"unhandled": "port-component",
        "mac": "mac-address",
        "ip": "network-address",
        "ifname": "interface-name",
        #"unhandled": "agent-circuit-id"
        "local": "local"
    }

    # lldpcli capability names, in the bit order of system-capabilities-map
    capability_bits = {
        "Other": "other",
        "Repeater": "repeater",
        "Bridge": "bridge",
        "Wlan": "wlan-access-point",
        "Router": "router",
        "Telephone": "telephone",
        "Docsis": "docsis-cable-device",
        "Station": "station-only",
    }

    LLDP_MULTICAST_MAC = "01:80:C2:00:00:0E"

    port_data = defaultdict(lambda: {"remote-systems-data": [], "dest-mac-address": LLDP_MULTICAST_MAC})

    data = HOST.run_json(["lldpcli", "show", "neighbors", "-f", "json"], {})

    interfaces = data.get("lldp", {}).get("interface", [])

    if isinstance(interfaces, dict):
        interfaces = [interfaces]

    seen_keys = defaultdict(set)

    for iface_entry in interfaces:
        for iface_name, iface_data in iface_entry.items():
            remote_index = int(iface_data.get("rid", 0))
            time_mark = parse_time(iface_data.get("age"))

            # lldpd's rid is per remote chassis, so one chassis heard on
            # two of its ports collides when the ages match too.  Nudge
            # time-mark to keep the YANG list keys unique, rid stays true
            # to lldpcli output.
            while (time_mark, remote_index) in seen_keys[iface_name]:
                time_mark += 1
            seen_keys[iface_name].add((time_mark, remote_index))

            chassis = iface_data.get("chassis", {})
            chassis_id_type, chassis_id_value = extract_chassis_id(chassis, chassis_id_subtype_mapping)
            system_name, chassis_info = chassis_system(chassis)

            port_info = iface_data.get("port", {})
            port_id_type = port_id_subtype_mapping.get(port_info.get("id", {}).get("type"), "unknown")
            port_id_value = port_info.get("id", {}).get("value", "")

            remote_entry = {
                "time-mark": time_mark,
                "remote-index": remote_index,
                "chassis-id-subtype": chassis_id_type,
                "chassis-id": chassis_id_value,
                "port-id-subtype": port_id_type,
                "port-id": port_id_value
            }
            if system_name:
                remote_entry["system-name"] = system_name
            if chassis_info.get("descr"):
                remote_entry["system-description"] = chassis_info["descr"]
            if port_info.get("descr"):
                remote_entry["port-desc"] = port_info["descr"]

            supported, enabled = capabilities(chassis_info.get("capability", []), capability_bits)
            if supported:
                remote_entry["system-capabilities-supported"] = supported
            if enabled:
                remote_entry["system-capabilities-enabled"] = enabled

            port_data[iface_name]["remote-systems-data"].append(remote_entry)

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

def chassis_system(chassis_block):
    """lldpcli keys the chassis block by system name when it knows one.
    Returns the name, or "", and the block with the chassis details."""
    if "id" in chassis_block:
        return "", chassis_block

    for name, value in chassis_block.items():
        if isinstance(value, dict):
            return name, value

    return "", {}

def capabilities(entries, bits):
    """Supported and enabled capability bits, as the model's space
    separated bit names in bit order."""
    if isinstance(entries, dict):
        entries = [entries]

    supported = [bits[e["type"]] for e in entries if e.get("type") in bits]
    enabled = [bits[e["type"]] for e in entries if e.get("type") in bits and e.get("enabled")]
    order = list(bits.values())
    return (" ".join(sorted(supported, key=order.index)),
            " ".join(sorted(enabled, key=order.index)))

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
