"""
Fetch LLDP local system data from remote device.
"""

reverse_subtype_mapping = {
    "chassis-component": 1,
    "interface-alias": 2,
    "port-component": 3,
    "mac-address": 4,
    "network-address": 5,
    "interface-name": 6,
    "local": 7  # 'local' maps to 7 as per LLDP spec
}

def get_remote_systems_data(target, port):
    """Fetch the full remote-systems-data list for a specific port"""
    content = target.get_data("/ieee802-dot1ab-lldp:lldp")

    if not content:
        return []

    for port_entry in content.get("lldp", {}).get("port", []):
        if port_entry.get("name") == port:
            return port_entry.get("remote-systems-data", [])

    return []

def get_chassis_ids(target, port):
    """Fetch all LLDP chassis IDs for neighbors on a specific port"""
    neighbors = get_remote_systems_data(target, port)
    return [neighbor.get("chassis-id") for neighbor in neighbors if "chassis-id" in neighbor]

def get_chassis_ids_subtype(target, port):
    """Fetch all LLDP chassis ID subtypes for neighbors on a specific port and convert them to numbers."""
    neighbors = get_remote_systems_data(target, port)

    return [
        reverse_subtype_mapping.get(neighbor.get("chassis-id-subtype"), 0)  # Default to 0 if unknown
        for neighbor in neighbors if "chassis-id-subtype" in neighbor
    ]
