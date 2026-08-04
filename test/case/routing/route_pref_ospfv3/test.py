#!/usr/bin/env python3
"""Route preference: OSPFv3 vs Static

This test configures a device with both an OSPFv3-acquired route on a
dedicated interface and a static route to the same IPv6 destination on
another interface. The static route has a higher preference value than
OSPFv3.

Initially, the device should prefer the OSPFv3 route; if the OSPFv3 route
becomes unavailable, the static route should take over.

Note: OSPFv3 has no IPv4 address to derive a router-id from, so an
explicit-router-id is configured on every router.
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel
from infamy.netns import TPMR

OSPFV3 = "infix-routing:ospfv3"


def configure_interface(name, ip, prefix_length, forwarding=True):
    return {
        "name": name,
        "enabled": True,
        "ipv6": {
            "forwarding": forwarding,
            "address": [{"ip": ip, "prefix-length": prefix_length}]
        }
    }


def config_target1(target, data, link, ospf):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    configure_interface(data, "2001:db8:10::1", 64),
                    configure_interface(link, "2001:db8:50::1", 64),
                    configure_interface(ospf, "2001:db8:60::1", 64)
                ]
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [
                        {
                            "type": OSPFV3,
                            "name": "ospf-default",
                            "ospf": {
                                "explicit-router-id": "1.1.1.1",
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
                                "ipv6": {
                                    "route": [{
                                        "destination-prefix": "2001:db8:20::/64",
                                        "next-hop": {"next-hop-address": "2001:db8:50::2"},
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
                    configure_interface(data, "2001:db8:20::2", 64),
                    configure_interface(link, "2001:db8:50::2", 64),
                    configure_interface(ospf, "2001:db8:60::2", 64)
                ]
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [
                        {
                            "type": OSPFV3,
                            "name": "ospf-default",
                            "ospf": {
                                "explicit-router-id": "2.2.2.2",
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
                                "ipv6": {
                                    "route": [{
                                        "destination-prefix": "::/0",
                                        "next-hop": {"next-hop-address": "2001:db8:50::1"}
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
        ns1.addip("2001:db8:10::11", prefix_length=64, proto="ipv6")
        ns1.addroute("default", "2001:db8:10::1", proto="ipv6")

        ns2 = infamy.IsolatedMacVlan(hport_data2).start()
        ns2.addip("2001:db8:20::22", prefix_length=64, proto="ipv6")
        ns2.addroute("default", "2001:db8:20::2", proto="ipv6")

    with test.step("Wait for OSPFv3 and static routes"):
        print("Waiting for OSPFv3 and static routes...")
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:20::/64", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:20::/64", proto="ietf-routing:static"), attempts=200)

    with test.step("Verify connectivity from PC:data1 to PC:data2 via OSPFv3"):
        ns1.must_reach("2001:db8:20::22")

        ospf_route_active = route.ipv6_route_exist(R1, "2001:db8:20::/64", proto="ietf-ospf:ospfv3", active_check=True)
        assert ospf_route_active, "OSPFv3 route should be preferred when available."

        hops = [row[1] for row in ns1.traceroute("2001:db8:20::22")]
        assert "2001:db8:60::2" in hops, f"Path does not use expected OSPFv3 route: {hops}"

    with test.step("Simulate OSPFv3 route loss by blocking OSPFv3 interface"):
        ospf_breaker.block()
        until(lambda: not route.ipv6_route_exist(R1, "2001:db8:20::/64", proto="ietf-ospf:ospfv3"), attempts=200)

    with test.step("Verify connectivity via static route after OSPFv3 failover"):
        ns1.must_reach("2001:db8:20::22")

        static_route_active = route.ipv6_route_exist(R1, "2001:db8:20::/64", proto="ietf-routing:static", active_check=True)
        assert static_route_active, "Static route should be preferred when OSPFv3 route is unavailable."

        hops = [row[1] for row in ns1.traceroute("2001:db8:20::22")]
        assert "2001:db8:50::2" in hops, f"Path does not use expected static route: {hops}"

    test.succeed()
