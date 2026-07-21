#!/usr/bin/env python3
"""RIPng Passive Interface

Verifies RIPng passive interface functionality.  A passive interface means
that RIPng will include the interface's network in routing updates but will
not send or receive RIPng updates on that interface.

R1 has two RIPng-enabled interfaces:
- data: Passive interface (2001:db8:10::/64 advertised but no updates sent/received)
- link: Active interface (RIPng updates exchanged with R2)

R2 should learn about 2001:db8:10::/64 from R1 via the link interface, even
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
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:10::1",
                            "prefix-length": 64
                        }]}
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
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:50::2",
                            "prefix-length": 64
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

    with test.step("Wait for RIPng to exchange routes"):
        print("Waiting for RIPng routes to propagate...")
        # R2 should learn about R1's passive interface network (2001:db8:10::/64)
        # even though it's passive, R1 should still advertise it
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:10::/64", proto="ietf-rip:rip"), attempts=40)

    with test.step("Verify connectivity to passive interface network"):
        # Test that we can reach the passive interface from PC
        _, hport0 = env.ltop.xlate("PC", "data")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip("2001:db8:10::2", prefix_length=64, proto="ipv6")
            # No need for route since we're on the same network
            ns0.must_reach("2001:db8:10::1")

    test.succeed()
