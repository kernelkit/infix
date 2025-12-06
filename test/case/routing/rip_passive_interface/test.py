#!/usr/bin/env python3
"""RIP Passive Interface

Verifies RIP passive interface functionality.  A passive interface means that
RIP will include the interface's network in routing updates but will not send
or receive RIP updates on that interface.

R1 has two RIP-enabled interfaces:
- data: Passive interface (192.168.10.0/24 advertised but no updates sent/received)
- link: Active interface (RIP updates exchanged with R2)

R2 should learn about 192.168.10.0/24 from R1 via the link interface, even
though the data interface is passive.

"""

import infamy
import infamy.route as route
from infamy.util import until, parallel


def config_target1(target, data, link):
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
                        }]}
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
                            "interfaces": {
                                "interface": [{
                                    "interface": data,
                                    "passive": None
                                }, {
                                    "interface": link
                                }]
                            }
                        }
                    }]
                }
            }
        }
    })


def config_target2(target, link):
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

    with test.step("Configure targets"):
        _, R1data = env.ltop.xlate("R1", "data")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R1link = env.ltop.xlate("R1", "link")

        parallel(config_target1(R1, R1data, R1link),
                 config_target2(R2, R2link))

    with test.step("Wait for RIP to exchange routes"):
        print("Waiting for RIP routes to propagate...")
        # R2 should learn about R1's passive interface network (192.168.10.0/24)
        # even though it's passive, R1 should still advertise it
        until(lambda: route.ipv4_route_exist(R2, "192.168.10.0/24", proto="ietf-rip:rip"), attempts=40)

    with test.step("Verify connectivity to passive interface network"):
        # Test that we can reach the passive interface from PC
        _, hport0 = env.ltop.xlate("PC", "data")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip("192.168.10.2")
            # No need for route since we're on the same network
            ns0.must_reach("192.168.10.1")

    test.succeed()
