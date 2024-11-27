#!/usr/bin/env python3
"""
Route preference: OSPF vs Static 

This test configures a device with both an OSPF-acquired route on a
dedicated interface and a static route to the same destination on
another interface. The static route has a higher preference value than
OSPF.

Initially, the device should prefer the OSPF route; if the OSPF route 
becomes unavailable, the static route should take over.
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel
from infamy.netns import TPMR


def configure_interface(name, ip, prefix_length, forwarding=True):
    return {
        "name": name,
        "enabled": True,
        "ipv4": {
            "forwarding": forwarding,
            "address": [{"ip": ip, "prefix-length": prefix_length}]
        }
    }

def config_target1(target, data, link, ospf):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    configure_interface(data, "192.168.10.1", 24),
                    configure_interface(link, "192.168.50.1", 24),
                    configure_interface(ospf, "192.168.60.1", 24)
                ]
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [
                        {
                            "type": "infix-routing:ospfv2",
                            "name": "ospf-default",
                            "ospf": {
                                "redistribute": {
                                    "redistribute": [{"protocol": "connected"}]
                                },
                                "areas": {
                                    "area": [{
                                        "area-id": "0.0.0.0",
                                        "interfaces": {
                                            "interface": [{
                                                "name": ospf,
                                                "hello-interval": 1,
                                                "dead-interval": 3
                                            }]
                                        }
                                    }]
                                }
                            }
                        },
                        {
                            "type": "infix-routing:static",
                            "name": "dot20",
                            "static-routes": {
                                "ipv4": {
                                    "route": [{
                                        "destination-prefix": "192.168.20.0/24",
                                        "next-hop": {"next-hop-address": "192.168.50.2"},
                                        "route-preference": 120
                                    }]
                                }
                            }
                        }
                    ]
                }
            }
        }
    })

def config_target2(target, data, link, ospf):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    configure_interface(data, "192.168.20.2", 24),
                    configure_interface(link, "192.168.50.2", 24),
                    configure_interface(ospf, "192.168.60.2", 24)
                ]
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [
                        {
                            "type": "infix-routing:ospfv2",
                            "name": "ospf-default",
                            "ospf": {
                                "redistribute": {
                                    "redistribute": [{"protocol": "connected"}]
                                },
                                "areas": {
                                    "area": [{
                                        "area-id": "0.0.0.0",
                                        "interfaces": {
                                            "interface": [{
                                                "name": ospf,
                                                "hello-interval": 1,
                                                "dead-interval": 3
                                            }]
                                        }
                                    }]
                                }
                            }
                        },
                        {
                            "type": "infix-routing:static",
                            "name": "default",
                            "static-routes": {
                                "ipv4": {
                                    "route": [{
                                        "destination-prefix": "0.0.0.0/0",
                                        "next-hop": {"next-hop-address": "192.168.50.1"}
                                    }]
                                }
                            }
                        }
                    ]
                }
            }
        }
    })

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        R1 = env.attach("R1", "mgmt")
        R2 = env.attach("R2", "mgmt")

    with test.step("Set up TPMR between R1ospf and R2ospf"):
        ospf_breaker = TPMR(env.ltop.xlate("PC", "R1_ospf")[1], env.ltop.xlate("PC", "R2_ospf")[1]).start()

    with test.step("Configure targets"):
        _, R1data = env.ltop.xlate("R1", "data")
        _, R1link = env.ltop.xlate("R1", "link")
        _, R1ospf = env.ltop.xlate("R1", "ospf")
        _, R2data = env.ltop.xlate("R2", "data")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R2ospf = env.ltop.xlate("R2", "ospf")

        parallel(config_target1(R1, R1data, R1link, R1ospf), config_target2(R2, R2data, R2link, R2ospf))

    with test.step("Set up persistent MacVlan namespaces"):
        _, hport_data1 = env.ltop.xlate("PC", "data1")
        _, hport_data2 = env.ltop.xlate("PC", "data2")

        ns1 = infamy.IsolatedMacVlan(hport_data1).start()
        ns1.addip("192.168.10.11", prefix_length=24)
        ns1.addroute("default", "192.168.10.1")

        ns2 = infamy.IsolatedMacVlan(hport_data2).start()
        ns2.addip("192.168.20.22", prefix_length=24)
        ns2.addroute("default", "192.168.20.2")

    with test.step("Wait for OSPF and static routes"):
        print("Waiting for OSPF and static routes...")
        until(lambda: route.ipv4_route_exist(R1, "192.168.20.0/24", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R1, "192.168.20.0/24", proto="ietf-routing:static"), attempts=200)

    with test.step("Verify connectivity from PC:data1 to PC:data2 via OSPF"):
        ns1.must_reach("192.168.20.22")

        ospf_route_active = route.ipv4_route_exist(R1, "192.168.20.0/24", proto="ietf-ospf:ospfv2", active_check=True)
        assert ospf_route_active, "OSPF route should be preferred when available."

        hops = [row[1] for row in ns1.traceroute("192.168.20.22")]
        assert "192.168.60.2" in hops, f"Path does not use expected OSPF route: {hops}"

    with test.step("Simulate OSPF route loss by blocking OSPF interface"):
        ospf_breaker.block()
        until(lambda: not route.ipv4_route_exist(R1, "192.168.20.0/24", proto="ietf-ospf:ospfv2"), attempts=200)

    with test.step("Verify connectivity via static route after OSPF failover"):
        ns1.must_reach("192.168.20.22")

        static_route_active = route.ipv4_route_exist(R1, "192.168.20.0/24", proto="ietf-routing:static", active_check=True)
        assert static_route_active, "Static route should be preferred when OSPF route is unavailable."

        hops = [row[1] for row in ns1.traceroute("192.168.20.22")]
        assert "192.168.50.2" in hops, f"Path does not use expected static route: {hops}"

    test.succeed()
