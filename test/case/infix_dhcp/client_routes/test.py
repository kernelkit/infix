#!/usr/bin/env python3
"""DHCP option 121 vs option 3

Verify that DHCP option 121 (classless static routes) is used over the
older option 3 (default gateway) in the DHCP client, when both are sent
by the server, see RFC3442.  Use the routing RIB in the operational
datastore to verify.

Installing routes from a DHCP server should not affect already existing
(static) routes.  To verify this, a static canary route (20.0.0.0/24 via
192.168.0.2) is installed before starting the DHCP client.  As a twist,
this canary route has a next-hop (192.168.0.2) which is not reachable
until a DHCP lease has been acquired.

The DHCP server is set up to hand out both a default router (option 3)
via 192.168.0.1 and a default route (option 121) via 192.168.0.254.
"""

import infamy
import infamy.dhcp
import infamy.route as route
from infamy.util import until

with infamy.Test() as test:
    PREFIX = '10.0.0.0/24'
    ROUTER = '192.168.0.254'
    CANARY = '20.0.0.0/24'
    CANHOP = '192.168.0.2'

    with test.step("Setting up canary route, 20.0.0.0/24 via 192.168.0.2"):
        env = infamy.Env()
        client = env.attach("client", "mgmt")
        _, host = env.ltop.xlate("host", "data")

        # Install canary route to smoke out any regressions in
        # how the DHCP client installs routes in the kernel FIB
        client.put_config_dict("ietf-routing", {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": "infix-routing:static",
                        "name": "default",
                        "static-routes": {
                            "ipv4": {
                                "route": [{
                                    "destination-prefix": CANARY,
                                    "next-hop": {
                                        "next-hop-address": CANHOP
                                    },
                                    "infix-routing:route-preference": 250
                                }]
                            }
                        }
                    }]
                }
            }
        })

    with test.step("Enabling DHCP client, allow option 3 and 121"):
        _, port = env.ltop.xlate("client", "data")

        client.put_config_dict("infix-dhcp-client", {
            "dhcp-client": {
                "client-if": [{
                    "if-name": f"{port}",
                    "option": [
                        {"id": "router"},
                        {"id": "classless-static-route"}
                    ]
                }]
            }
        })

    with infamy.IsolatedMacVlan(host) as netns:
        netns.addip("192.168.0.1")
        print(f"Start DHCP server {ROUTER} with option 3 and 121")
        with infamy.dhcp.Server(netns, prefix=PREFIX, router=ROUTER):
            with test.step("Verify client has route 10.0.0.0/24 via 192.168.0.254 (option 121)"):
                print("Verify client use classless routes, option 121")
                until(lambda: route.ipv4_route_exist(client, PREFIX, ROUTER))

            with test.step("Verify client has default route via 192.168.0.254 (not use option 3)"):
                print("Verify client did *not* use option 3")
                if route.ipv4_route_exist(client, "0.0.0.0/0", ROUTER):
                    test.fail()

            with test.step("Verify client still has canary route to 20.0.0.0/24 via 192.168.0.2"):
                until(lambda: route.ipv4_route_exist(client, CANARY,
                                                CANHOP, pref=250))

    test.succeed()
