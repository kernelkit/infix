#!/usr/bin/env python3
"""RIP Multi-hop

Verifies RIP functionality across multiple hops with three routers in a line
topology (R1 -- R2 -- R3). This test ensures:
- RIP routes propagate through multiple hops
- R2 (middle router) has two RIP neighbors
- End-to-end connectivity works across the RIP network

Topology:
  PC:data1 -- R1 -- R2 -- R3 -- PC:data2
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
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.10.1",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": link,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.50.1",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": "lo",
                    "enabled": True,
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.11.1",
                            "prefix-length": 32
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
                        "type": "infix-routing:ripv2",
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
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.50.2",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": east,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.60.1",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": "lo",
                    "enabled": True,
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.22.1",
                            "prefix-length": 32
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
                        "type": "infix-routing:ripv2",
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
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.60.2",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": data,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.70.1",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": "lo",
                    "enabled": True,
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.33.1",
                            "prefix-length": 32
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
                        "type": "infix-routing:ripv2",
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

    with test.step("Wait for RIP routes to be exchanged"):
        print("Waiting for RIP routes to propagate...")
        # R1 should learn R2's loopback
        until(lambda: route.ipv4_route_exist(R1, "192.168.22.1/32", proto="ietf-rip:rip"), attempts=40)
        # R1 should learn R3's loopback (via R2)
        until(lambda: route.ipv4_route_exist(R1, "192.168.33.1/32", proto="ietf-rip:rip"), attempts=40)
        # R2 should learn R1's loopback
        until(lambda: route.ipv4_route_exist(R2, "192.168.11.1/32", proto="ietf-rip:rip"), attempts=40)
        # R2 should learn R3's loopback
        until(lambda: route.ipv4_route_exist(R2, "192.168.33.1/32", proto="ietf-rip:rip"), attempts=40)
        # R3 should learn R2's loopback
        until(lambda: route.ipv4_route_exist(R3, "192.168.22.1/32", proto="ietf-rip:rip"), attempts=40)
        # R3 should learn R1's loopback (via R2)
        until(lambda: route.ipv4_route_exist(R3, "192.168.11.1/32", proto="ietf-rip:rip"), attempts=40)

    with test.step("Verify R2 has two RIP neighbors"):
        print("Checking R2 has two RIP neighbors...")
        # R2 should have neighbors: 192.168.50.1 (R1) and 192.168.60.2 (R3)
        # Query without predicates to avoid RESTCONF encoding issues
        routing_data = R2.get_data("/ietf-routing:routing/control-plane-protocols")

        # Navigate to RIP protocol
        protocols = routing_data.get("routing", {}).get("control-plane-protocols", {}).get("control-plane-protocol", [])
        if not protocols:
            raise Exception("No protocols found")

        # Find RIP protocol
        rip = None
        for protocol in protocols:
            if protocol.get("type") == "infix-routing:ripv2" and protocol.get("name") == "default":
                rip = protocol.get("rip", {})
                break

        if not rip:
            raise Exception("RIP protocol not found in control-plane-protocols")

        ipv4_data = rip.get("ipv4", {})
        neighbors_data = ipv4_data.get("neighbors", {})
        neighbor_list = neighbors_data.get("neighbor", [])

        assert len(neighbor_list) == 2, f"Expected 2 neighbors, found {len(neighbor_list)}"

        neighbor_ips = [n.get("ipv4-address") for n in neighbor_list]
        assert "192.168.50.1" in neighbor_ips, "R1 not in neighbor list"
        assert "192.168.60.2" in neighbor_ips, "R3 not in neighbor list"
        print(f"R2 has correct neighbors: {neighbor_ips}")

    with test.step("Test end-to-end connectivity PC:data1 to R3 loopback"):
        _, hport1 = env.ltop.xlate("PC", "data1")
        with infamy.IsolatedMacVlan(hport1) as ns1:
            ns1.addip("192.168.10.2")
            ns1.addroute("192.168.33.1/32", "192.168.10.1")
            ns1.must_reach("192.168.33.1")

    with test.step("Test end-to-end connectivity PC:data2 to R1 loopback"):
        _, hport2 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport2) as ns2:
            ns2.addip("192.168.70.2")
            ns2.addroute("192.168.11.1/32", "192.168.70.1")
            ns2.must_reach("192.168.11.1")

    test.succeed()
