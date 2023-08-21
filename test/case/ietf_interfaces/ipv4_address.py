#!/usr/bin/env python3

import copy

import infamy

#Set of parameters needed to add/remove IPv4 addresses from interface
new_ip_address = "10.10.10.20"
new_prefix_length = 24

def print_ip_addresses(target):
    running = target.get_config_dict("/ietf-interfaces:interfaces")
    interfaces = running.get("interfaces", {}).get("interface", [])

    for interface in interfaces:
        name = interface["name"]

        if "ipv4" in interface:
            ipv4_addresses = interface["ipv4"].get("address", [])
            for ipv4_address in ipv4_addresses:
                address = ipv4_address.get("ip")
                prefix_length = ipv4_address.get("prefix-length")
                print(f"Interface: {name}, IPv4 Address: {address}/{prefix_length}")
        else:
            print(f"Interface: {name}, No IPv4 Address")

        if "ipv6" in interface:
            ipv6_addresses = interface["ipv6"].get("address", [])
            for ipv6_address in ipv6_addresses:
                address = ipv6_address.get("ip")
                prefix_length = ipv6_address.get("prefix-length")
                print(f"Interface: {name}, IPv6 Address: {address}/{prefix_length}")
        else:
            print(f"Interface: {name}, No IPv6 Address")

def assert_ipv4_address_in_interface(target, ip_address, interface_name, prefix_length):
    running = target.get_config_dict("/ietf-interfaces:interfaces")
    interfaces = running.get("interfaces", {}).get("interface", [])

    found = False

    for interface in interfaces:
        name = interface["name"]

        if name == interface_name:
            if "ipv4" in interface:
                ipv4_addresses = interface["ipv4"].get("address", [])
                for ipv4_address in ipv4_addresses:
                    address = ipv4_address.get("ip")
                    length = ipv4_address.get("prefix-length")
                    if address == ip_address and length == prefix_length:
                        found = True
                        break

    assert found, f"IP address {ip_address}/{prefix_length} not found for interface {interface_name}"

def assert_ipv4_removed(target, interface_name):
    running = target.get_config_dict("/ietf-interfaces:interfaces")
    interfaces = running.get("interfaces", {}).get("interface", [])

    for interface in interfaces:
        name = interface["name"]
        if name == interface_name:
            assert "ipv4" not in interface, f"IPv4 address not removed from interface {interface_name}"
            return
    print(f"No interface with the name {interface_name}")

with infamy.Test() as test:
    with test.step("Setup"):
        env = infamy.Env(infamy.std_topology("1x1"))
        target = env.attach("target", "mgmt")
        _, interface_name = env.ltop.xlate("target", "mgmt")

    with test.step("Get initial IP addresses"):
        print_ip_addresses(target)

    with test.step("Configure IP address"):
        config = {
            "interfaces": {
                "interface": [{
                    "name": f"{interface_name}",
                    "type": "infix-if-type:ethernet",
                    "ipv4": {
                        "address": [{
                            "ip": f"{new_ip_address}",
                            "prefix-length": new_prefix_length
                        }]
                    }
                }]
            }
        }

        target.put_config_dict("ietf-interfaces", config)

    with test.step("Get updated IP addresses"):
        print_ip_addresses(target)
        assert_ipv4_address_in_interface(target, new_ip_address, interface_name, new_prefix_length)

    with test.step(f"Remove IPv4 addresses from {interface_name}"):
        # Get current interface configuration
        running = target.get_config_dict("/ietf-interfaces:interfaces")

        new = copy.deepcopy(running)
        for iface in new["interfaces"]["interface"]:
            if iface["name"] == interface_name and "ipv4" in iface:
                del iface["ipv4"]
                break

        target.put_diff_dicts("ietf-interfaces", running, new)

    with test.step("Get updated IP addresses"):
        print_ip_addresses(target)
        assert_ipv4_removed(target, interface_name)

    test.succeed()
