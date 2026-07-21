#!/usr/bin/env python3
"""RIPng Multi-hop

Verifies RIPng functionality across multiple hops with three routers in a line
topology (R1 -- R2 -- R3). This test ensures:
- RIPng routes propagate through multiple hops
- R2 (middle router) has two RIPng neighbors
- End-to-end connectivity works across the RIPng network over IPv6

Topology:
  PC:data1 -- R1 -- R2 -- R3 -- PC:data2

Note: RIPng peers using IPv6 link-local (fe80::) addresses, so unlike the
RIPv2 multi-hop test we assert the neighbor *count* rather than specific
neighbor addresses.
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel


def config_r1(target, data, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": data,
                    "enabled": True,
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:10::1",
                            "prefix-length": 64
                        }]
                    }
                }, {
                    "name": link,
                    "enabled": True,
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:50::1",
                            "prefix-length": 64
                        }]
                    }
                }, {
                    "name": "lo",
                    "enabled": True,
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:11::1",
                            "prefix-length": 128
                        }]
                    }
                }]
            }
        },
        "ietf-system": {
            "system": {
                "hostname": "R1"
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": "infix-routing:ripng",
                        "name": "default",
                        "rip": {
                            "timers": {
                                "update-interval": 5,
                                "invalid-interval": 15,
                                "flush-interval": 20
                            },
                            "redistribute": {
                                "redistribute": [{
                                    "protocol": "connected"
                                }]
                            },
                            "interfaces": {
                                "interface": [{
                                    "interface": link
                                }]
                            }
                        }
                    }]
                }
            }
        }
    })


def config_r2(target, west, east):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": west,
                    "enabled": True,
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:50::2",
                            "prefix-length": 64
                        }]
                    }
                }, {
                    "name": east,
                    "enabled": True,
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:60::1",
                            "prefix-length": 64
                        }]
                    }
                }, {
                    "name": "lo",
                    "enabled": True,
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:22::1",
                            "prefix-length": 128
                        }]
                    }
                }]
            }
        },
        "ietf-system": {
            "system": {
                "hostname": "R2"
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": "infix-routing:ripng",
                        "name": "default",
                        "rip": {
                            "timers": {
                                "update-interval": 5,
                                "invalid-interval": 15,
                                "flush-interval": 20
                            },
                            "redistribute": {
                                "redistribute": [{
                                    "protocol": "connected"
                                }]
                            },
                            "interfaces": {
                                "interface": [{
                                    "interface": west
                                }, {
                                    "interface": east
                                }]
                            }
                        }
                    }]
                }
            }
        }
    })


def config_r3(target, link, data):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": link,
                    "enabled": True,
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:60::2",
                            "prefix-length": 64
                        }]
                    }
                }, {
                    "name": data,
                    "enabled": True,
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:70::1",
                            "prefix-length": 64
                        }]
                    }
                }, {
                    "name": "lo",
                    "enabled": True,
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:33::1",
                            "prefix-length": 128
                        }]
                    }
                }]
            }
        },
        "ietf-system": {
            "system": {
                "hostname": "R3"
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": "infix-routing:ripng",
                        "name": "default",
                        "rip": {
                            "timers": {
                                "update-interval": 5,
                                "invalid-interval": 15,
                                "flush-interval": 20
                            },
                            "redistribute": {
                                "redistribute": [{
                                    "protocol": "connected"
                                }]
                            },
                            "interfaces": {
                                "interface": [{
                                    "interface": link
                                }]
                            }
                        }
                    }]
                }
            }
        }
    })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        R1 = env.attach("R1", "mgmt")
        R2 = env.attach("R2", "mgmt")
        R3 = env.attach("R3", "mgmt")

    with test.step("Configure routers"):
        _, R1data = env.ltop.xlate("R1", "data")
        _, R1link = env.ltop.xlate("R1", "link")
        _, R2west = env.ltop.xlate("R2", "west")
        _, R2east = env.ltop.xlate("R2", "east")
        _, R3link = env.ltop.xlate("R3", "link")
        _, R3data = env.ltop.xlate("R3", "data")

        parallel(config_r1(R1, R1data, R1link),
                 config_r2(R2, R2west, R2east),
                 config_r3(R3, R3link, R3data))

    with test.step("Wait for RIPng routes to be exchanged"):
        print("Waiting for RIPng routes to propagate...")
        # R1 should learn R2's loopback
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:22::1/128", proto="ietf-rip:rip"), attempts=40)
        # R1 should learn R3's loopback (via R2)
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:33::1/128", proto="ietf-rip:rip"), attempts=40)
        # R2 should learn R1's loopback
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:11::1/128", proto="ietf-rip:rip"), attempts=40)
        # R2 should learn R3's loopback
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:33::1/128", proto="ietf-rip:rip"), attempts=40)
        # R3 should learn R2's loopback
        until(lambda: route.ipv6_route_exist(R3, "2001:db8:22::1/128", proto="ietf-rip:rip"), attempts=40)
        # R3 should learn R1's loopback (via R2)
        until(lambda: route.ipv6_route_exist(R3, "2001:db8:11::1/128", proto="ietf-rip:rip"), attempts=40)

    with test.step("Verify R2 has two RIPng neighbors"):
        print("Checking R2 has two RIPng neighbors...")
        # RIPng peers via IPv6 link-local (fe80::) addresses, which are not
        # deterministic, so we assert the neighbor count rather than addresses.
        routing_data = R2.get_data("/ietf-routing:routing/control-plane-protocols")

        protocols = routing_data.get("routing", {}).get("control-plane-protocols", {}).get("control-plane-protocol", [])
        if not protocols:
            raise Exception("No protocols found")

        rip = None
        for protocol in protocols:
            if protocol.get("type") == "infix-routing:ripng" and protocol.get("name") == "default":
                rip = protocol.get("rip", {})
                break

        if not rip:
            raise Exception("RIPng protocol not found in control-plane-protocols")

        ipv6_data = rip.get("ipv6", {})
        neighbors_data = ipv6_data.get("neighbors", {})
        neighbor_list = neighbors_data.get("neighbor", [])

        assert len(neighbor_list) == 2, f"Expected 2 neighbors, found {len(neighbor_list)}"
        print(f"R2 has 2 RIPng neighbors: {[n.get('ipv6-address') for n in neighbor_list]}")

    with test.step("Test end-to-end connectivity PC:data1 to R3 loopback"):
        _, hport1 = env.ltop.xlate("PC", "data1")
        with infamy.IsolatedMacVlan(hport1) as ns1:
            ns1.addip("2001:db8:10::2", prefix_length=64, proto="ipv6")
            ns1.addroute("2001:db8:33::1/128", "2001:db8:10::1", proto="ipv6")
            ns1.must_reach("2001:db8:33::1")

    with test.step("Test end-to-end connectivity PC:data2 to R1 loopback"):
        _, hport2 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport2) as ns2:
            ns2.addip("2001:db8:70::2", prefix_length=64, proto="ipv6")
            ns2.addroute("2001:db8:11::1/128", "2001:db8:70::1", proto="ipv6")
            ns2.must_reach("2001:db8:11::1")

    test.succeed()
