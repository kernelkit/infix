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
        xpath=target.get_iface_xpath(interface_name, path="ietf-ip:ipv4")
        target.delete_xpath(xpath)
    with test.step("Get updated IP addresses"):
        until(lambda: iface.address_exist(target, interface_name, new_ip_address) == False)

    test.succeed()
