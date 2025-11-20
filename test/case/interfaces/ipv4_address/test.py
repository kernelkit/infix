#!/usr/bin/env python3
"""
Interface with IPv4

Test that it is possible to set and remove the IPv4 address on an interface
"""

import infamy
import infamy.iface as iface

from infamy.util import until

#Set of parameters needed to add/remove IPv4 addresses from interface
new_ip_address = "10.10.10.20"
new_prefix_length = 24

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, interface_name = env.ltop.xlate("target", "mgmt")

    with test.step("Configure IPv4 address 10.10.10.20/24 on target:mgmt"):
        print(f"Initial IPv4 address for target:mgmt {iface.get_ipv4_address(target, interface_name)}")

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

    with test.step("Verify '10.10.10.20/24' exists on target:mgmt"):
        until(lambda: iface.address_exist(target, interface_name, new_ip_address, proto='static'))

    with test.step("Remove all IPv4 addresses from target:mgmt"):
        target.delete_xpath(f"/ietf-interfaces:interfaces/interface[name='{interface_name}']/ietf-ip:ipv4")

    with test.step("Verify target:mgmt no longer has the address 10.10.10.20"):
        until(lambda: iface.address_exist(target, interface_name, new_ip_address) == False)

    test.succeed()
