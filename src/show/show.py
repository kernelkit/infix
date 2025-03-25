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

    if len(args) == 0:
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

    if len(args) == 0:
        cli_pretty(data, "show-software")
        print(".....")
    elif len(args) == 1:
        name = args[0]
        if name not in ("primary", "secondary"):
            print(f"Invalid software name: {name}")
            return

        cli_pretty(data, f"show-software", "-n", name)
    else:
        print("Too many arguments provided. Only one name is expected.")

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


def execute_command(command: str, args: List[str]):
    command_mapping = {
        'dhcp': dhcp,
        'hardware': hardware,
        'interface': interface,
        'ntp': ntp,
        'routes': routes,
        'software' : software,
        'stp': stp,
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
