import re
from .host import HOST


def parse_rip_status():
    """Parse 'show ip rip status' text output to extract operational state

    Returns dict with keys: update-interval, invalid-interval, flush-interval,
                           default-metric, distance, interfaces (list), neighbors (list)
    """
    try:
        # HOST.run expects tuple, returns text string directly
        text = HOST.run(tuple(['vtysh', '-c', 'show ip rip status']), default="")
        if not text:
            return {}
    except Exception as e:
        return {}

    status = {}

    # Parse: "Sending updates every 30 seconds"
    match = re.search(r'Sending updates every (\d+) seconds', text)
    if match:
        status['update-interval'] = int(match.group(1))

    # Parse: "Timeout after 180 seconds"
    match = re.search(r'Timeout after (\d+) seconds', text)
    if match:
        status['invalid-interval'] = int(match.group(1))

    # Parse: "garbage collect after 240 seconds"
    match = re.search(r'garbage collect after (\d+) seconds', text)
    if match:
        status['flush-interval'] = int(match.group(1))

    # Parse: "Default redistribution metric is 1"
    match = re.search(r'Default redistribution metric is (\d+)', text)
    if match:
        status['default-metric'] = int(match.group(1))

    # Parse: "Distance: (default is 120)"
    match = re.search(r'Distance: \(default is (\d+)\)', text)
    if match:
        status['distance'] = int(match.group(1))

    # Parse interface table:
    #     Interface        Send  Recv   Key-chain
    #     e5               2     2
    interfaces = []
    in_interface_section = False
    for line in text.split('\n'):
        line = line.strip()

        # Detect start of interface section
        if 'Interface' in line and 'Send' in line and 'Recv' in line:
            in_interface_section = True
            continue

        # Stop at next section
        if in_interface_section and (line.startswith('Routing for Networks:') or
                                      line.startswith('Routing Information Sources:')):
            break

        # Parse interface lines
        if in_interface_section and line:
            # Format: "e5               2     2      "
            parts = line.split()
            if len(parts) >= 3 and not line.startswith('Interface'):
                iface_name = parts[0]
                send_version = parts[1]
                recv_version = parts[2]

                interfaces.append({
                    'name': iface_name,
                    'send-version': int(send_version),
                    'recv-version': int(recv_version)
                })

    if interfaces:
        status['interfaces'] = interfaces

    # Parse Routing Information Sources table:
    #     Gateway          BadPackets BadRoutes  Distance Last Update
    #     192.168.50.2              0         0       120    00:00:09
    neighbors = []
    in_neighbor_section = False
    for line in text.split('\n'):
        line = line.strip()

        # Detect start of Routing Information Sources section
        if line.startswith('Routing Information Sources:'):
            in_neighbor_section = True
            continue

        # Skip the header line
        if in_neighbor_section and 'Gateway' in line and 'BadPackets' in line:
            continue

        # Stop at next section (Distance line or empty lines after table)
        if in_neighbor_section and (line.startswith('Distance:') or
                                     (not line and neighbors)):
            break

        # Parse neighbor lines
        if in_neighbor_section and line:
            # Format: "192.168.50.2              0         0       120    00:00:09"
            parts = line.split()
            if len(parts) >= 5:
                try:
                    gateway = parts[0]
                    bad_packets = int(parts[1])
                    bad_routes = int(parts[2])
                    distance = int(parts[3])
                    last_update = parts[4]  # Format: HH:MM:SS

                    neighbors.append({
                        'address': gateway,
                        'bad-packets': bad_packets,
                        'bad-routes': bad_routes,
                        'distance': distance,
                        'last-update': last_update
                    })
                except (ValueError, IndexError):
                    # Skip lines that don't parse correctly
                    continue

    if neighbors:
        status['neighbors'] = neighbors

    return status


def add_rip(control_protocols):
    """Populate RIP operational data

    Note: FRR's RIP implementation provides JSON for routing table but not for
    status commands, so we combine JSON route data with text parsing for status.
    """
    # Get operational status from text parsing
    status = parse_rip_status()

    # Check if RIP is running - if we can't get status, it's probably not running
    if not status:
        return

    control_protocol = {}
    control_protocol["type"] = "infix-routing:ripv2"
    control_protocol["name"] = "default"
    control_protocol["ietf-rip:rip"] = {}

    rip = control_protocol["ietf-rip:rip"]

    # Add global operational state
    if status.get('distance'):
        rip['distance'] = status['distance']
    if status.get('default-metric'):
        rip['default-metric'] = status['default-metric']

    # Add timers if available
    if any(k in status for k in ['update-interval', 'invalid-interval', 'flush-interval']):
        rip['timers'] = {}
        if status.get('update-interval'):
            rip['timers']['update-interval'] = status['update-interval']
        if status.get('invalid-interval'):
            rip['timers']['invalid-interval'] = status['invalid-interval']
        if status.get('flush-interval'):
            rip['timers']['flush-interval'] = status['flush-interval']

    # Add interfaces if available
    if status.get('interfaces'):
        rip['interfaces'] = {'interface': []}
        for iface in status['interfaces']:
            iface_data = {
                'interface': iface['name'],
                'oper-status': 'up'  # If it's in the list, it's operational
            }

            # Map FRR version numbers to YANG enum values
            # FRR shows: 1=v1, 2=v2, and we assume if both are enabled it would show differently
            send_ver = iface.get('send-version')
            if send_ver == 1:
                iface_data['send-version'] = '1'
            elif send_ver == 2:
                iface_data['send-version'] = '2'
            # Note: FRR might show this differently for mixed mode

            recv_ver = iface.get('recv-version')
            if recv_ver == 1:
                iface_data['receive-version'] = '1'
            elif recv_ver == 2:
                iface_data['receive-version'] = '2'

            rip['interfaces']['interface'].append(iface_data)

    # Get RIP-learned routes from routing table (JSON)
    # This shows routes learned via RIP (R(n) entries), not redistributed routes
    route_data = HOST.run_json(['vtysh', '-c', 'show ip route rip json'], default={})

    routes = []
    for prefix, entries in route_data.items():
        if not entries or '/' not in prefix:
            continue

        # Use first entry (RIP doesn't typically have ECMP)
        entry = entries[0] if isinstance(entries, list) else entries

        route = {
            "ipv4-prefix": prefix,
            "metric": entry.get("metric", 0),
            "route-type": "rip"
        }

        # Get next hop information
        nexthops = entry.get("nexthops", [])
        if nexthops:
            first_hop = nexthops[0]
            if first_hop.get("ip"):
                route["next-hop"] = first_hop["ip"]
            if first_hop.get("interfaceName"):
                route["interface"] = first_hop["interfaceName"]

        routes.append(route)

    # Add neighbors to operational data
    neighbors_list = []
    if status.get('neighbors'):
        for neighbor in status['neighbors']:
            neighbor_data = {
                'ipv4-address': neighbor['address'],
                'bad-packets-rcvd': neighbor['bad-packets'],
                'bad-routes-rcvd': neighbor['bad-routes']
            }
            # Note: IETF YANG expects last-update as yang:date-and-time
            # but FRR gives us a relative time like "00:00:09"
            # We'll skip last-update for now or convert it if needed
            neighbors_list.append(neighbor_data)

    # Add routes and neighbors to operational data
    if routes or neighbors_list:
        if "ipv4" not in rip:
            rip["ipv4"] = {}

        if routes:
            rip["ipv4"]["routes"] = {
                "route": routes
            }
            # Add route count
            rip["num-of-routes"] = len(routes)

        if neighbors_list:
            rip["ipv4"]["neighbors"] = {
                "neighbor": neighbors_list
            }

    # Add the control-protocol
    if "ietf-routing:control-plane-protocol" not in control_protocols:
        control_protocols["ietf-routing:control-plane-protocol"] = []
    control_protocols["ietf-routing:control-plane-protocol"].append(control_protocol)


def operational():
    """Return RIP operational data in YANG format"""
    out = {
        "ietf-routing:routing": {
            "control-plane-protocols": {}
        }
    }

    add_rip(out['ietf-routing:routing']['control-plane-protocols'])
    return out
