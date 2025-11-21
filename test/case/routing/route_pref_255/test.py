#!/usr/bin/env python3
"""
Route preference: Static Route Activation and Maximum Distance

This test configures a device with a static route to a destination with 
a moderate routing preference (254), verifying that it becomes active. 
Then, the routing preference is increased to the maximum value (255), 
which should prevent the route from becoming active.
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel

def configure_interface(name, ip, prefix_length, forwarding=True):
    return {
        "name": name,
        "enabled": True,
        "ipv4": {
            "forwarding": forwarding,
            "address": [{"ip": ip, "prefix-length": prefix_length}]
        }
    }

def config_target1_initial(target, data, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    configure_interface(data, "192.168.10.1", 24),
                    configure_interface(link, "192.168.50.1", 24)
                ]
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [
                        {
                            "type": "infix-routing:static",
                            "name": "default",
                            "static-routes": {
                                "ipv4": {
                                    "route": [{
                                        "destination-prefix": "192.168.20.0/24",
                                        "next-hop": {"next-hop-address": "192.168.50.2"},
                                        "route-preference": 254
                                    }]
                                }
                            }
                        }
                    ]
                }
            }
        }
    })

def config_target1_update(target):
    target.put_config_dicts({
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [
                        {
                            "type": "infix-routing:static",
                            "name": "default",
                            "static-routes": {
                                "ipv4": {
                                    "route": [{
                                        "destination-prefix": "192.168.20.0/24",
                                        "next-hop": {"next-hop-address": "192.168.50.2"},
                                        "route-preference": 255
                                    }]
                                }
                            }
                        }
                    ]
                }
            }
        }
    })

def config_target2(target, data, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    configure_interface(data, "192.168.20.2", 24),
                    configure_interface(link, "192.168.50.2", 24)
                ]
            }
        }
    })

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        R1 = env.attach("R1", "mgmt")
        R2 = env.attach("R2", "mgmt")

    with test.step("Configure targets with active static route"):
        _, R1data = env.ltop.xlate("R1", "data")
        _, R1link = env.ltop.xlate("R1", "link")
        _, R2data = env.ltop.xlate("R2", "data")
        _, R2link = env.ltop.xlate("R2", "link")

        parallel(config_target1_initial(R1, R1data, R1link), config_target2(R2, R2data, R2link))

    with test.step("Verify that static route with preference 254 is active"):
        until(lambda: route.ipv4_route_exist(R1, "192.168.20.0/24", proto="ietf-routing:static", active_check=True))

    with test.step("Update static route preference to 255"):
        config_target1_update(R1)

    with test.step("Verify that high-preference static route (255) does not become active"):
        until(lambda: not route.ipv4_route_exist(R1, "192.168.20.0/24", proto="ietf-routing:static", active_check=True))

    test.succeed()
