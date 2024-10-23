#!/usr/bin/env python3
"""
DHCP Basic

This is a very basic DHCP test that requests an IPv4 lease
from a DHCP server and checks that the lease is set on the
interface.
"""

import infamy, infamy.dhcp
import infamy.iface as iface
from infamy.util import until

with infamy.Test() as test:
    ADDRESS = '10.0.0.42'
    with test.step("Initialize"):
        env = infamy.Env()
        client = env.attach("client", "mgmt")
        _, host = env.ltop.xlate("host", "data")

    with infamy.IsolatedMacVlan(host) as netns:
        netns.addip("10.0.0.1")
        with infamy.dhcp.Server(netns, ip=ADDRESS):
            _, port = env.ltop.xlate("client", "data")
            config = {
                "dhcp-client": {
                    "client-if": [{
                        "if-name": f"{port}"
                    }]
                }
            }
            client.put_config_dict("infix-dhcp-client", config)

            with test.step("Verify client get DHCP lease for 10.0.0.42 on client:data"):
                until(lambda: iface.address_exist(client, port, ADDRESS))

    test.succeed()
