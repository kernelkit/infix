#!/usr/bin/env python3
"""RIP Redistribution

Verifies that RIP can redistribute routes from other protocols.

Topology:
- R1: Gateway router running both RIP and OSPF
  - RIP interface to R2
  - OSPF interface to R3
  - Redistributes OSPF routes into RIP
  - Redistributes RIP routes into OSPF

- R2: RIP-only router with loopback 192.168.200.1/32

- R3: OSPF-only router with loopback 192.168.100.1/32

Expected behavior:
- R2 (RIP) should learn R3's OSPF loopback (192.168.100.1/32) via RIP redistribution
- R3 (OSPF) should learn R2's RIP loopback (192.168.200.1/32) via OSPF redistribution

"""

import infamy
import infamy.route as route
from infamy.util import until, parallel


def config_r1_gateway(target, rip_link, ospf_link):
    """Configure R1 as gateway running both RIP and OSPF"""
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": rip_link,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.50.1",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": ospf_link,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.60.1",
                            "prefix-length": 24
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
                        "type": "infix-routing:ospfv2",
                        "name": "default",
                        "ospf": {
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
    """Configure R2 with RIP only"""
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": link,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.50.2",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": "lo",
                    "enabled": True,
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.200.1",
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
    """Configure R3 with OSPF only"""
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
                    "name": "lo",
                    "enabled": True,
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.100.1",
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
                        "type": "infix-routing:ospfv2",
                        "name": "default",
                        "ospf": {
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

    with test.step("Wait for OSPF to converge on R1-R3 link"):
        print("Waiting for OSPF convergence...")
        # R1 should learn R3's loopback via OSPF
        until(lambda: route.ipv4_route_exist(R1, "192.168.100.1/32", proto="ietf-ospf:ospfv2"), attempts=40)
        # R3 should learn R1's OSPF link via OSPF
        until(lambda: route.ipv4_route_exist(R3, "192.168.60.0/24", proto="ietf-ospf:ospfv2"), attempts=40)

    with test.step("Wait for RIP to converge on R1-R2 link"):
        print("Waiting for RIP convergence...")
        # R1 should learn R2's loopback via RIP
        until(lambda: route.ipv4_route_exist(R1, "192.168.200.1/32", proto="ietf-rip:rip"), attempts=40)
        # R2 should learn R1's OSPF link (192.168.60.0/24) via RIP (redistributed from connected)
        until(lambda: route.ipv4_route_exist(R2, "192.168.60.0/24", proto="ietf-rip:rip"), attempts=40)

    with test.step("Verify R2 (RIP) learns R3's OSPF routes via redistribution"):
        print("Checking OSPF->RIP redistribution...")
        # R2 should learn R3's loopback (OSPF route) via RIP redistribution on R1
        until(lambda: route.ipv4_route_exist(R2, "192.168.100.1/32", proto="ietf-rip:rip"), attempts=40)

    with test.step("Verify R3 (OSPF) learns R2's RIP routes via redistribution"):
        print("Checking RIP->OSPF redistribution...")
        # R3 should learn R2's loopback (RIP route) via OSPF redistribution on R1
        until(lambda: route.ipv4_route_exist(R3, "192.168.200.1/32", proto="ietf-ospf:ospfv2"), attempts=40)

    test.succeed()
