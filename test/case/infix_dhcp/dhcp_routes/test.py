#!/usr/bin/env python3
# Verify DHCP option 121 (staticroutes) is used over option 3

import time
import infamy, infamy.dhcp
import infamy.iface as iface
import infamy.route as route
from infamy.util import until

with infamy.Test() as test:
    PREFIX  = '10.0.0.0/24'
    ROUTER  = '192.168.0.254'
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, host = env.ltop.xlate("host", "data")

    with infamy.IsolatedMacVlan(host) as netns:
        netns.addip("192.168.0.1")
        with infamy.dhcp.Server(netns, prefix=PREFIX, router=ROUTER):
            _, port = env.ltop.xlate("target", "data")
            config = {
                "dhcp-client": {
                    "client-if": [{
                        "if-name": f"{port}",
                        "option": [
                            { "name": "router" },
                            { "name": "staticroutes" }
                        ]
                    }]
                }
            }
            target.put_config_dict("infix-dhcp-client", config)

            with test.step(f"Verify client sets up correct route via {ROUTER}"):
                # Wait for client to set the classless static route, option 121
                until(lambda: route.ipv4_route_exist(target, PREFIX, ROUTER))
                # Ensure client did *not* use option 3 (option 121 takes precedence)
                if route.ipv4_route_exist(target, "0.0.0.0/0", ROUTER):
                    test.fail()

    test.succeed()
