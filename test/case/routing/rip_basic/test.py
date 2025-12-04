#!/usr/bin/env python3
"""RIP Basic

Verifies basic RIP functionality by configuring two routers (R1 and R2)
with RIP on their interconnecting link.  The test ensures RIP routes are
exchanged between the routers and end-to-end connectivity is achieved.

The test PC uses data1 interface to connect to R1's data port, and data2
interface to connect to R2's data port (which does not have RIP enabled).
This verifies that RIP status information remains accessible when a router
has non-RIP interfaces.

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
                "hostname": "R1"
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": "infix-routing:static",
                        "name": "default",
                        "static-routes": {
                            "ipv4": {
                                "route": [{
                                    "destination-prefix": "192.168.33.1/32",
                                    "next-hop": {
                                        "special-next-hop": "blackhole"
                                    }
                                }]
                            }
                        }
                    }, {
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
                                    "protocol": "static"
                                }, {
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


def config_target2(target, link, data):
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
                    "name": data,
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
                    "forwarding": True,
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


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        R1 = env.attach("R1", "mgmt")
        R2 = env.attach("R2", "mgmt")

    with test.step("Configure targets"):
        _, R1data = env.ltop.xlate("R1", "data")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R1link = env.ltop.xlate("R1", "link")
        _, R2data = env.ltop.xlate("R2", "data")

        parallel(config_target1(R1, R1data, R1link),
                 config_target2(R2, R2link, R2data))

    with test.step("Wait for RIP routes to be exchanged"):
        print("Waiting for RIP routes to propagate...")
        # R1 should learn R2's loopback
        until(lambda: route.ipv4_route_exist(R1, "192.168.200.1/32", proto="ietf-rip:rip"), attempts=40)
        # R2 should learn R1's loopback
        until(lambda: route.ipv4_route_exist(R2, "192.168.100.1/32", proto="ietf-rip:rip"), attempts=40)
        # R2 should learn R1's static route (redistributed)
        until(lambda: route.ipv4_route_exist(R2, "192.168.33.1/32", proto="ietf-rip:rip"), attempts=40)

    with test.step("Test connectivity from PC:data1 to R2 loopback via RIP"):
        _, hport0 = env.ltop.xlate("PC", "data1")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip("192.168.10.2")
            ns0.addroute("192.168.200.1/32", "192.168.10.1")
            ns0.must_reach("192.168.200.1")

    with test.step("Test connectivity from PC:data2 to R1 loopback via RIP"):
        _, hport1 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport1) as ns1:
            ns1.addip("192.168.60.2")
            ns1.addroute("192.168.100.1/32", "192.168.60.1")
            ns1.must_reach("192.168.100.1")

    test.succeed()
