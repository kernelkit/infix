#!/usr/bin/env python3
"""
DHCP option 121 vs option 3

Verify DHCP option 121 (staticroutes) is used over option 3 and that the
routes exist in the operational datastore.

Installing unrelated routes from a DHCP server should not affect already
existing routes.  To verify this a canary route is set up in the client
before initiating DHCP.  This canary route does not need to be reachable
before a DHCP lease has been acquired.
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

    with test.step("Setting up client"):
        env = infamy.Env()
        client = env.attach("client", "mgmt")
        _, host = env.ltop.xlate("host", "data")
        _, port = env.ltop.xlate("client", "data")

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

        client.put_config_dict("infix-dhcp-client", {
            "dhcp-client": {
                "client-if": [{
                    "if-name": f"{port}",
                    "option": [
                        {"name": "router"},
                        {"name": "staticroutes"}
                    ]
                }]
            }
        })

    with infamy.IsolatedMacVlan(host) as netns:
        netns.addip("192.168.0.1")
        with infamy.dhcp.Server(netns, prefix=PREFIX, router=ROUTER):
            with test.step("Verify client use classless routes, option 121"):
                until(lambda: route.ipv4_route_exist(client, PREFIX, ROUTER))

            with test.step("Verify client did *not* use option 3"):
                if route.ipv4_route_exist(client, "0.0.0.0/0", ROUTER):
                    test.fail()

            with test.step("Verify client has canary route, 20.0.0.0/24"):
                until(lambda: route.ipv4_route_exist(client, CANARY,
                                                     CANHOP, pref=250))

    test.succeed()
