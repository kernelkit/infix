#!/usr/bin/python3
# This script is used to transform the output from the show ip rip commands
# to match the ietf-rip YANG model structure.
#
# This makes the parsing for the operational parts of YANG model more easy
#

import sys
import json
import subprocess


def run_json_cmd(cmd, default=None, check=True):
    """Run a command (array of args) with JSON output and return the JSON"""
    try:
        result = subprocess.run(cmd, check=check, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, text=True)
        output = result.stdout
        data = json.loads(output)
    except (subprocess.CalledProcessError, json.JSONDecodeError):
        if default is not None:
            return default
        raise
    return data


def main():
    """
    Collects RIP operational data from FRR vtysh and transforms it
    into a structure that matches the ietf-rip YANG model.
    """
    rip_routes_cmd = ['sudo', 'vtysh', '-c', "show ip rip json"]
    rip_status_cmd = ['sudo', 'vtysh', '-c', "show ip rip status json"]

    try:
        routes = run_json_cmd(rip_routes_cmd, default={})
        status = run_json_cmd(rip_status_cmd, default={})
    except (subprocess.CalledProcessError, json.JSONDecodeError):
        return {}

    # Build the structure matching ietf-rip YANG model
    result = {
        "routes": routes.get("routes", {}),
        "status": status
    }

    return result


if __name__ == "__main__":
    try:
        data = main()
        print(json.dumps(data, indent=2))
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
