#!/usr/bin/env python3
#
#   PC ---- e0: 10.0.0.2
#

import infamy
import time
import infamy.iface

from infamy.util import until

def has_linklocal(target, iface):
    """Check if interface as a linklocal address"""
    addrs = infamy.iface.get_ipv4_address(target, iface)
    if not addrs:
        return False
    for addr in addrs:
        if addr['origin'] == "random":
            return True

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x2"))
        target = env.attach("target", "mgmt")

    with test.step("Configure an interface with IPv4 ZeroConf IP"):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": tport,
                        "enabled": True,
                        "ipv4": {
                            "autoconf": {
                                "enabled": True
                            }
                        }
                    }
                ]
            }
        })

    with test.step("Wait for linklocal address on interface"):
         until(lambda: has_linklocal(target, tport), attempts=10)

    test.succeed()
