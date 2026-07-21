#!/usr/bin/env python3
"""RIPng Basic

Verifies basic RIPng (RIP for IPv6) functionality by configuring two routers
(R1 and R2) with RIPng on their interconnecting link.  The test ensures RIPng
routes are exchanged between the routers and end-to-end connectivity is
achieved over IPv6.

The test PC uses data1 interface to connect to R1's data port, and data2
interface to connect to R2's data port (which does not have RIPng enabled).
This verifies that RIPng status information remains accessible when a router
has non-RIPng interfaces.

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
                            "ipv6": {
                                "route": [{
                                    "destination-prefix": "2001:db8:33::1/128",
                                    "next-hop": {
                                        "special-next-hop": "blackhole"
                                    }
                                }]
                            }
                        }
                    }, {
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
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:50::2",
                            "prefix-length": 64
                        }]
                    }
                }, {
                    "name": data,
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
                            "ip": "2001:db8:200::1",
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

    with test.step("Wait for RIPng routes to be exchanged"):
        print("Waiting for RIPng routes to propagate...")
        # R1 should learn R2's loopback
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:200::1/128", proto="ietf-rip:rip"), attempts=40)
        # R2 should learn R1's loopback
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:100::1/128", proto="ietf-rip:rip"), attempts=40)
        # R2 should learn R1's static route (redistributed)
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:33::1/128", proto="ietf-rip:rip"), attempts=40)

    with test.step("Test connectivity from PC:data1 to R2 loopback via RIPng"):
        _, hport0 = env.ltop.xlate("PC", "data1")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip("2001:db8:10::2", prefix_length=64, proto="ipv6")
            ns0.addroute("2001:db8:200::1/128", "2001:db8:10::1", proto="ipv6")
            ns0.must_reach("2001:db8:200::1")

    with test.step("Test connectivity from PC:data2 to R1 loopback via RIPng"):
        _, hport1 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport1) as ns1:
            ns1.addip("2001:db8:60::2", prefix_length=64, proto="ipv6")
            ns1.addroute("2001:db8:100::1/128", "2001:db8:60::1", proto="ipv6")
            ns1.must_reach("2001:db8:100::1")

    test.succeed()
