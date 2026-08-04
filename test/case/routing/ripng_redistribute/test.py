#!/usr/bin/env python3
"""RIPng Redistribution

Verifies that RIPng and OSPFv3 can redistribute routes between each other over
IPv6, mirroring the RIPv2/OSPFv2 redistribution test (this variant requires
OSPFv3 support).

Topology:
- R1: Gateway (ASBR) running both RIPng and OSPFv3
  - RIPng interface to R2
  - OSPFv3 interface to R3
  - Redistributes OSPFv3 routes into RIPng (redistribute ospf -> ospf6)
  - Redistributes RIPng routes into OSPFv3 (redistribute rip -> ripng)

- R2: RIPng-only router with loopback 2001:db8:200::1/128

- R3: OSPFv3-only router with loopback 2001:db8:100::1/128

Expected behavior:
- R2 (RIPng) learns R3's OSPFv3 loopback (2001:db8:100::1/128) via redistribution.
- R3 (OSPFv3) learns R2's RIPng loopback (2001:db8:200::1/128) via redistribution.

Note: OSPFv3 has no IPv4 address to derive a router-id from, so an
explicit-router-id is configured on the OSPFv3 routers.
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel


def config_r1_gateway(target, rip_link, ospf_link):
    """Configure R1 as gateway running both RIPng and OSPFv3"""
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": rip_link,
                    "enabled": True,
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:50::1",
                            "prefix-length": 64
                        }]
                    }
                }, {
                    "name": ospf_link,
                    "enabled": True,
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:60::1",
                            "prefix-length": 64
                        }]
                    }
                }]
            }
        },
        "ietf-system": {
            "system": {"hostname": "R1"}
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
                                    "protocol": "ospf"
                                }, {
                                    "protocol": "connected"
                                }]
                            },
                            "interfaces": {
                                "interface": [{
                                    "interface": rip_link
                                }]
                            }
                        }
                    }, {
                        "type": "infix-routing:ospfv3",
                        "name": "default",
                        "ospf": {
                            "explicit-router-id": "1.1.1.1",
                            "redistribute": {
                                "redistribute": [{
                                    "protocol": "rip"
                                }, {
                                    "protocol": "connected"
                                }]
                            },
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.0",
                                    "interfaces": {
                                        "interface": [{
                                            "enabled": True,
                                            "name": ospf_link,
                                            "hello-interval": 1,
                                            "dead-interval": 3
                                        }]
                                    }
                                }]
                            }
                        }
                    }]
                }
            }
        }
    })


def config_r2_rip(target, link):
    """Configure R2 with RIPng only"""
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": link,
                    "enabled": True,
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:50::2",
                            "prefix-length": 64
                        }]
                    }
                }, {
                    "name": "lo",
                    "enabled": True,
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:200::1",
                            "prefix-length": 128
                        }]
                    }
                }]
            }
        },
        "ietf-system": {
            "system": {"hostname": "R2"}
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


def config_r3_ospf(target, link):
    """Configure R3 with OSPFv3 only"""
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
                    "name": "lo",
                    "enabled": True,
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:100::1",
                            "prefix-length": 128
                        }]
                    }
                }]
            }
        },
        "ietf-system": {
            "system": {"hostname": "R3"}
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": "infix-routing:ospfv3",
                        "name": "default",
                        "ospf": {
                            "explicit-router-id": "3.3.3.3",
                            "redistribute": {
                                "redistribute": [{
                                    "protocol": "connected"
                                }]
                            },
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.0",
                                    "interfaces": {
                                        "interface": [{
                                            "enabled": True,
                                            "name": link,
                                            "hello-interval": 1,
                                            "dead-interval": 3
                                        }]
                                    }
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
        _, R1rip = env.ltop.xlate("R1", "rip")
        _, R1ospf = env.ltop.xlate("R1", "ospf")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R3link = env.ltop.xlate("R3", "link")

        parallel(config_r1_gateway(R1, R1rip, R1ospf),
                 config_r2_rip(R2, R2link),
                 config_r3_ospf(R3, R3link))

    with test.step("Wait for OSPFv3 to converge on R1-R3 link"):
        print("Waiting for OSPFv3 convergence...")
        # R1 should learn R3's loopback via OSPFv3
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:100::1/128", proto="ietf-ospf:ospfv3"), attempts=40)
        # R3 should learn R1's OSPFv3 link via OSPFv3
        until(lambda: route.ipv6_route_exist(R3, "2001:db8:60::/64", proto="ietf-ospf:ospfv3"), attempts=40)

    with test.step("Wait for RIPng to converge on R1-R2 link"):
        print("Waiting for RIPng convergence...")
        # R1 should learn R2's loopback via RIPng
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:200::1/128", proto="ietf-rip:rip"), attempts=40)
        # R2 should learn R1's OSPFv3 link (2001:db8:60::/64) via RIPng (redistributed connected)
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:60::/64", proto="ietf-rip:rip"), attempts=40)

    with test.step("Verify R2 (RIPng) learns R3's OSPFv3 routes via redistribution"):
        print("Checking OSPFv3->RIPng redistribution...")
        # R2 should learn R3's loopback (OSPFv3 route) via RIPng redistribution on R1
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:100::1/128", proto="ietf-rip:rip"), attempts=40)

    with test.step("Verify R3 (OSPFv3) learns R2's RIPng routes via redistribution"):
        print("Checking RIPng->OSPFv3 redistribution...")
        # R3 should learn R2's loopback (RIPng route) via OSPFv3 redistribution on R1
        until(lambda: route.ipv6_route_exist(R3, "2001:db8:200::1/128", proto="ietf-ospf:ospfv3"), attempts=40)

    test.succeed()
