#!/usr/bin/env python3

import re
import subprocess
import json
from typing import List
import os
import shlex
import argparse

RAW_OUTPUT = False

def run_sysrepocfg(xpath: str) -> dict:
    if not isinstance(xpath, str) or not xpath.startswith("/"):
        print("Invalid XPATH. It must be a valid string starting with '/'.")
        return {}

    safe_xpath = shlex.quote(xpath)

    try:
        result = subprocess.run([
            "sysrepocfg", "-f", "json", "-X", "-d", "operational", "-x", safe_xpath
        ], capture_output=True, text=True, check=True)
        json_data = json.loads(result.stdout)
        return json_data
    except subprocess.CalledProcessError as e:
        print(f"Error running sysrepocfg: {e}")
        return {}
    except json.JSONDecodeError as e:
        print(f"Error parsing JSON output: {e}")
        return {}

def cli_pretty(json_data: dict, command: str, *args: str):
    if not command or not all(isinstance(arg, str) for arg in args):
        print("Invalid command or arguments. All arguments must be strings.")
        return

    safe_args = [shlex.quote(arg) for arg in args]

    try:
        json_input = json.dumps(json_data)  # Keep as string, not bytes
        result = subprocess.run([
            "/usr/libexec/statd/cli-pretty", command, *safe_args
        ], input=json_input, capture_output=True, text=True, check=True)
        print(result.stdout, end="")
    except subprocess.CalledProcessError as e:
        print(f"Error running cli-pretty: {e}")

def dhcp(args: List[str]) -> None:
    data = run_sysrepocfg("/infix-dhcp-server:dhcp-server")
    if not data:
        print("No interface data retrieved.")
        return

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return
    cli_pretty(data, "show-dhcp-server")

def hardware(args: List[str]) -> None:
    data = run_sysrepocfg("/ietf-hardware:hardware")
    if not data:
        print("No hardware data retrieved.")
        return

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return
    cli_pretty(data, "show-hardware")

def ntp(args: List[str]) -> None:
    data = run_sysrepocfg("/system-state/ntp")
    if not data:
        print("No ntp data retrieved.")
        return

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return
    cli_pretty(data, "show-ntp")

def is_valid_interface_name(interface_name: str) -> bool:
    """
    Validates a Linux network interface name.
    """
    if len(interface_name) > 15:
        return False

    pattern = r'^[a-zA-Z0-9._-]+$'
    return bool(re.match(pattern, interface_name))

def interface(args: List[str]) -> None:
    data = run_sysrepocfg("/ietf-interfaces:interfaces")
    if not data:
        print("No interface data retrieved.")
        return

    # Also fetch routing interface list for forwarding indication
    routing_data = run_sysrepocfg("/ietf-routing:routing")
    if routing_data:
        # Merge routing data into the main data structure
        data.update(routing_data)

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return

    if len(args) == 0 or not args[0]:  # Treat "" as no arg.
        cli_pretty(data, "show-interfaces")
    elif len(args) == 1:
        iface = args[0]
        if is_valid_interface_name(iface):
            cli_pretty(data, f"show-interfaces", "-n", iface)
        else:
            print(f"Invalid interface name: {iface}")
    else:
        print("Too many arguments provided. Only one interface name is expected.")

def stp(args: List[str]) -> None:
    data = run_sysrepocfg("/ietf-interfaces:interfaces")
    if not data:
        print("No interface data retrieved.")
        return

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return
    cli_pretty(data, "show-bridge-stp")

def software(args: List[str]) -> None:
    data = run_sysrepocfg("/ietf-system:system-state/infix-system:software")
    if not data:
        print("No software data retrieved.")
        return

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return
    if len(args) == 0 or not args[0]:  # Treat "" as no arg.
        cli_pretty(data, "show-software")
    elif len(args) == 1:
        name = args[0]
        if name not in ("primary", "secondary"):
            print(f"Invalid software name: {name}")
            return

        cli_pretty(data, f"show-software", "-n", name)
    else:
        print("Too many arguments provided. Only one name is expected.")

def boot_order(args: List[str]) -> None:
    data = run_sysrepocfg("/ietf-system:system-state/infix-system:software")
    if not data:
        print("No software data retrieved.")
        return

    try:
        boot_order_list = data.get("ietf-system:system-state", {}).get("infix-system:software", {}).get("boot-order", [])
        if boot_order_list:
            print(" ".join(boot_order_list))
    except (KeyError, TypeError):
        print("No boot order data available.")

def services(args: List[str]) -> None:
    data = run_sysrepocfg("/ietf-system:system-state/infix-system:services")
    if not data:
        print("No service data retrieved.")
        return

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return

    cli_pretty(data, f"show-services")

def container(args: List[str]) -> None:
    """Handle show container [name]

    Arguments:
        (none) - Show all containers in table format
        name   - Show detailed view of specific container
    """
    data = run_sysrepocfg("/infix-containers:containers")
    if not data:
        print("No container data retrieved.")
        return

    # Fetch interface data for bridge resolution (both table and detailed views)
    # Fetch operational interface data
    iface_oper = run_sysrepocfg("/ietf-interfaces:interfaces")

    # Also fetch config data for veth peer information (not in operational)
    try:
        result = subprocess.run([
            "sysrepocfg", "-f", "json", "-X", "-d", "running", "-x", "/ietf-interfaces:interfaces"
        ], capture_output=True, text=True, check=True)
        iface_config = json.loads(result.stdout)

        # Merge config veth peer info into operational data
        if iface_oper and iface_config:
            oper_ifaces = iface_oper.get('ietf-interfaces:interfaces', {}).get('interface', [])
            config_ifaces = iface_config.get('ietf-interfaces:interfaces', {}).get('interface', [])

            # Create a map of config interfaces
            config_map = {iface['name']: iface for iface in config_ifaces}

            # Merge veth peer info from config into operational
            for oper_iface in oper_ifaces:
                name = oper_iface.get('name')
                if name in config_map:
                    config_iface = config_map[name]
                    # Add veth peer if it exists in config but not in operational
                    if 'infix-interfaces:veth' in config_iface and 'infix-interfaces:veth' not in oper_iface:
                        oper_iface['infix-interfaces:veth'] = config_iface['infix-interfaces:veth']

            data.update(iface_oper)
    except (subprocess.CalledProcessError, json.JSONDecodeError):
        # If config fetch fails, just use operational data
        if iface_oper:
            data.update(iface_oper)

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return

    if len(args) == 0 or not args[0]:
        cli_pretty(data, "show-container")
    elif len(args) == 1:
        name = args[0]
        cli_pretty(data, "show-container-detail", name)
    else:
        print("Too many arguments provided. Expected: show container [name]")

def bfd(args: List[str]) -> None:
    """Handle show bfd [subcommand] [peer] [brief]

    Subcommands:
        (none)      - Show BFD status summary (default)
        peers       - Show BFD peer sessions
        peers brief - Show BFD peers in brief format
        peer <addr> - Show specific BFD peer details
    """
    # Fetch operational data from sysrepocfg
    data = run_sysrepocfg("/ietf-routing:routing/control-plane-protocols/control-plane-protocol")
    if not data:
        print("No BFD data retrieved.")
        return

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return

    # Parse arguments: [subcommand] [peer_addr] [brief]
    subcommand = ""
    peer_addr = None
    brief_flag = False

    for arg in args:
        if arg == "brief":
            brief_flag = True
        elif arg in ["peers", "peer"]:
            subcommand = arg
        elif arg:  # Must be peer address
            peer_addr = arg

    # Default to empty string for general status
    if not subcommand and not peer_addr:
        subcommand = ""

    # Add metadata to data
    if peer_addr:
        data['_peer_addr'] = peer_addr
    if brief_flag:
        data['_brief'] = True

    # For brief view, fetch interface data to show interface:ip format
    if brief_flag:
        ifaces_data = run_sysrepocfg("/ietf-interfaces:interfaces")
        if ifaces_data:
            data['_interfaces'] = ifaces_data

    # Route to appropriate formatter
    if subcommand == "":
        cli_pretty(data, "show-bfd-status")
    elif subcommand == "peers":
        if brief_flag:
            cli_pretty(data, "show-bfd-peers-brief")
        else:
            cli_pretty(data, "show-bfd-peers")
    elif subcommand == "peer":
        cli_pretty(data, "show-bfd-peer")
    else:
        print(f"Unknown BFD subcommand: {subcommand}")

def ospf(args: List[str]) -> None:
    """Handle show ospf [subcommand] [ifname] [detail]

    Subcommands:
        (none)      - General OSPF instance information
        neighbor    - OSPF neighbor table
        interfaces  - OSPF interface details (optionally for specific interface)
        routes      - OSPF routing table (local-rib)

    Optional:
        ifname      - Interface name (for interfaces subcommand)
        detail      - Show detailed information (Cisco-style)
    """
    # Fetch operational data from sysrepocfg
    data = run_sysrepocfg("/ietf-routing:routing/control-plane-protocols/control-plane-protocol")
    if not data:
        print("No OSPF data retrieved.")
        return

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return

    # Parse arguments: subcommand, optional interface name, optional detail flag
    subcommand = args[0] if len(args) > 0 and args[0] else ""

    # Determine if second arg is interface name or detail flag
    ifname = None
    detail_flag = False

    if len(args) > 1:
        if args[1] == "detail":
            detail_flag = True
        else:
            ifname = args[1]
            # Check if third arg is detail
            if len(args) > 2 and args[2] == "detail":
                detail_flag = True

    # Add metadata to data for formatters
    if detail_flag:
        data['_detail'] = True
    if ifname:
        data['_ifname'] = ifname
        # For detailed interface view, fetch additional data (interfaces and BFD)
        if subcommand == "interface" or subcommand == "interfaces":
            ifaces_data = run_sysrepocfg("/ietf-interfaces:interfaces")
            if ifaces_data:
                data['_interfaces'] = ifaces_data

            # Fetch BFD data for per-interface status
            routing_data = run_sysrepocfg("/ietf-routing:routing/control-plane-protocols/control-plane-protocol")
            if routing_data:
                data['_routing'] = routing_data

    # Route to appropriate formatter
    if subcommand == "":
        cli_pretty(data, "show-ospf")
    elif subcommand == "neighbor":
        cli_pretty(data, "show-ospf-neighbor")
    elif subcommand == "interface" or subcommand == "interfaces":
        cli_pretty(data, "show-ospf-interfaces")
    elif subcommand == "route" or subcommand == "routes":
        cli_pretty(data, "show-ospf-routes")
    else:
        print(f"Unknown OSPF subcommand: {subcommand}")

def rip(args: List[str]) -> None:
    """Handle show rip [subcommand] [ifname]

    Subcommands:
        (none)      - General RIP instance information
        route       - RIP routing table
        interface   - RIP interface details (optionally for specific interface)
        neighbor    - RIP neighbor information

    Optional:
        ifname      - Interface name (for interface subcommand)
    """
    # Fetch operational data from sysrepocfg
    data = run_sysrepocfg("/ietf-routing:routing/control-plane-protocols/control-plane-protocol")
    if not data:
        data = {}

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return

    # Parse arguments: subcommand, optional interface name
    subcommand = args[0] if len(args) > 0 and args[0] else ""
    ifname = args[1] if len(args) > 1 else None

    # Add metadata to data for formatters
    if ifname:
        data['_ifname'] = ifname

    # Route to appropriate formatter
    if subcommand == "":
        cli_pretty(data, "show-rip")
    elif subcommand == "route" or subcommand == "routes":
        cli_pretty(data, "show-rip-routes")
    elif subcommand == "interface" or subcommand == "interfaces":
        cli_pretty(data, "show-rip-interfaces")
    elif subcommand == "neighbor" or subcommand == "neighbors":
        cli_pretty(data, "show-rip-neighbors")
    else:
        print(f"Unknown RIP subcommand: {subcommand}")

def routes(args: List[str]):
    ip_version = args[0] if args and args[0] in ["ipv4", "ipv6"] else "ipv4"

    data = run_sysrepocfg("/ietf-routing:routing/ribs")
    if not data:
        print("No route data retrieved.")
        return
    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return
    cli_pretty(data, "show-routing-table", "-i", ip_version)

def lldp(args: List[str]):
    data = run_sysrepocfg("/ieee802-dot1ab-lldp:lldp")
    if not data:
        print("No lldp data retrieved.")
        return
    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return
    cli_pretty(data, "show-lldp")

def wifi(args: List[str]):
    iface = args[0]
    if len(args) == 0:
        print("Illigal usage")
        return
    if is_valid_interface_name(iface):
        if not os.path.exists(f"/sys/class/net/{iface}/wireless"):
            print("Not a Wi-Fi interface")
            return
        data = run_sysrepocfg("/ietf-interfaces:interfaces")
        cli_pretty(data, "show-wifi-scan", "-n", iface)
    else:
        print(f"Invalid interface name: {iface}")

def system(args: List[str]) -> None:
    # Get system state from sysrepo
    data = run_sysrepocfg("/ietf-system:system-state")
    if not data:
        print("No system data retrieved.")
        return

    # Get hardware data (including thermal sensors)
    hardware_data = run_sysrepocfg("/ietf-hardware:hardware")

    # Augment with runtime data
    runtime = {}

    # Extract CPU temperature and fan speed from hardware components
    cpu_temp = None
    fan_rpm = None
    if hardware_data and "ietf-hardware:hardware" in hardware_data:
        components = hardware_data.get("ietf-hardware:hardware", {}).get("component", [])
        for component in components:
            sensor_data = component.get("sensor-data", {})
            if not sensor_data:
                continue

            name = component.get("name", "")
            value_type = sensor_data.get("value-type")

            # Only capture CPU temperature (ignore phy, sfp, etc.)
            if value_type == "celsius" and name == "cpu" and cpu_temp is None:
                temp_millidegrees = sensor_data.get("value", 0)
                cpu_temp = temp_millidegrees / 1000.0

            # Capture fan speed if available
            elif value_type == "rpm" and fan_rpm is None:
                fan_rpm = sensor_data.get("value", 0)

    if cpu_temp is not None:
        runtime["cpu_temp"] = cpu_temp
    if fan_rpm is not None:
        runtime["fan_rpm"] = fan_rpm

    # Extract resource usage from system-state
    system_state = data.get("ietf-system:system-state", {})
    resource_usage = system_state.get("infix-system:resource-usage", {})

    # Memory info - convert KiB to MiB for display
    memory_kib = resource_usage.get("memory", {})
    if memory_kib:
        memory = {}
        if "total" in memory_kib:
            memory["MemTotal"] = int(memory_kib["total"]) // 1024
        if "free" in memory_kib:
            memory["MemFree"] = int(memory_kib["free"]) // 1024
        if "available" in memory_kib:
            memory["MemAvailable"] = int(memory_kib["available"]) // 1024
        runtime["memory"] = memory

    # Load average
    load_avg = resource_usage.get("load-average", {})
    if load_avg:
        runtime["load"] = {
            "1min": str(load_avg.get("load-1min", "0.00")),
            "5min": str(load_avg.get("load-5min", "0.00")),
            "15min": str(load_avg.get("load-15min", "0.00"))
        }

    # Filesystem usage - convert KiB to human-readable format
    filesystems = resource_usage.get("filesystem", [])
    if filesystems:
        disk_usage = []
        for fs in filesystems:
            mount = fs.get("mount-point", "")
            size_kib = int(fs.get("size", 0))
            used_kib = int(fs.get("used", 0))
            avail_kib = int(fs.get("available", 0))

            # Convert KiB to human-readable format (similar to df -h)
            def human_readable(kib_val):
                for unit in ['K', 'M', 'G', 'T']:
                    if kib_val < 1024.0:
                        return f"{kib_val:.1f}{unit}"
                    kib_val /= 1024.0
                return f"{kib_val:.1f}P"

            # Calculate percentage
            percent = f"{int((used_kib / size_kib * 100) if size_kib > 0 else 0)}%"

            disk_usage.append({
                "mount": mount,
                "size": human_readable(size_kib),
                "used": human_readable(used_kib),
                "available": human_readable(avail_kib),
                "percent": percent
            })
        runtime["disk"] = disk_usage

    # Add runtime data to main data structure
    data["runtime"] = runtime

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return

    cli_pretty(data, "show-system")

def execute_command(command: str, args: List[str]):
    command_mapping = {
        'bfd': bfd,
        'boot-order': boot_order,
        'container': container,
        'dhcp': dhcp,
        'hardware': hardware,
        'interface': interface,
        'lldp': lldp,
        'ntp': ntp,
        'ospf': ospf,
        'rip': rip,
        'routes': routes,
        'services': services,
        'software': software,
        'stp': stp,
        'system': system,
        'wifi': wifi
    }

    if command in command_mapping:
        command_mapping[command](args)
    else:
        print(f"Unknown command: {command}")


def main():
    global RAW_OUTPUT

    parser = argparse.ArgumentParser(description="Show operational data")
    parser.add_argument('command', help="Command to execute")
    parser.add_argument('args', nargs=argparse.REMAINDER, help="Additional arguments for the command")
    parser.add_argument('-r', '--raw', action='store_true', help="Print raw JSON output from Sysrepo")

    args = parser.parse_args()
    RAW_OUTPUT = args.raw

    execute_command(args.command, args.args)


if __name__ == "__main__":
    main()
