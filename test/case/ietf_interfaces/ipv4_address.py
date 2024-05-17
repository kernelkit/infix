#!/usr/bin/env python3

import copy
import infamy
import infamy.iface as iface

from infamy.util import until

#Set of parameters needed to add/remove IPv4 addresses from interface
new_ip_address = "10.10.10.20"
new_prefix_length = 24

with infamy.Test() as test:
    with test.step("Setup"):
        env = infamy.Env(infamy.std_topology("1x1"))
        target = env.attach("target", "mgmt")
        _, interface_name = env.ltop.xlate("target", "mgmt")

    with test.step("Get initial IP addresses"):
        print(iface.get_ipv4_address(target, interface_name))

    with test.step("Configure IP address"):
        config = {
            "interfaces": {
                "interface": [{
                    "name": f"{interface_name}",
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
        until(lambda: iface.address_exist(target, interface_name, new_ip_address, proto='static'))

    with test.step(f"Remove IPv4 addresses from {interface_name}"):
        # Get current interface configuration
        running = target.get_config_dict("/ietf-interfaces:interfaces")

        new = copy.deepcopy(running)
        for i in new["interfaces"]["interface"]:
            if i["name"] == interface_name and "ipv4" in i:
                del i["ipv4"]
                break

        target.put_diff_dicts("ietf-interfaces", running, new)

    with test.step("Get updated IP addresses"):
        until(lambda: iface.address_exist(target, interface_name, new_ip_address) == False)

    test.succeed()
