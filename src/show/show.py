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

def services(args: List[str]) -> None:
    data = run_sysrepocfg("/ietf-system:system-state/infix-system:services")
    if not data:
        print("No service data retrieved.")
        return

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return

    cli_pretty(data, f"show-services")

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

    # Augment with runtime data
    runtime = {}

    # Get thermal zones
    thermal_zones = []
    try:
        for zone in os.listdir("/sys/class/thermal"):
            if zone.startswith("thermal_zone"):
                try:
                    with open(f"/sys/class/thermal/{zone}/type") as f:
                        zone_type = f.read().strip()
                    with open(f"/sys/class/thermal/{zone}/temp") as f:
                        temp = int(f.read().strip()) / 1000.0
                    thermal_zones.append({"type": zone_type, "temp": temp})
                except (FileNotFoundError, ValueError):
                    pass
    except FileNotFoundError:
        pass

    if thermal_zones:
        runtime["thermal"] = thermal_zones

    # Get disk usage for /, /var, /cfg
    disk_usage = []
    for mount in ["/", "/var", "/cfg"]:
        try:
            result = subprocess.run(["df", "-h", mount],
                                    capture_output=True, text=True, check=True)
            lines = result.stdout.strip().split("\n")
            if len(lines) > 1:
                parts = lines[1].split()
                if len(parts) >= 5:
                    disk_usage.append({
                        "mount": mount,
                        "size": parts[1],
                        "used": parts[2],
                        "available": parts[3],
                        "percent": parts[4]
                    })
        except subprocess.CalledProcessError:
            pass

    if disk_usage:
        runtime["disk"] = disk_usage

    # Get memory info
    try:
        with open("/proc/meminfo") as f:
            mem_info = {}
            for line in f:
                parts = line.split(":")
                if len(parts) == 2:
                    key = parts[0].strip()
                    value = parts[1].strip()
                    if key in ["MemTotal", "MemFree", "MemAvailable"]:
                        # Convert from kB to MB
                        mem_info[key] = int(value.split()[0]) // 1024
            runtime["memory"] = mem_info
    except FileNotFoundError:
        pass

    # Get load average
    try:
        with open("/proc/loadavg") as f:
            load_parts = f.read().strip().split()
            if len(load_parts) >= 3:
                runtime["load"] = {
                    "1min": load_parts[0],
                    "5min": load_parts[1],
                    "15min": load_parts[2]
                }
    except FileNotFoundError:
        pass

    # Add runtime data to main data structure
    data["runtime"] = runtime

    if RAW_OUTPUT:
        print(json.dumps(data, indent=2))
        return

    cli_pretty(data, "show-system")

def execute_command(command: str, args: List[str]):
    command_mapping = {
        'dhcp': dhcp,
        'hardware': hardware,
        'interface': interface,
        'ntp': ntp,
        'routes': routes,
        'lldp': lldp,
        'services' : services,
        'software' : software,
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
