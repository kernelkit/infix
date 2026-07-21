#!/usr/bin/env python3
"""OSPFv3 Basic

Verifies basic OSPFv3 (OSPF for IPv6) functionality by configuring two routers
(R1 and R2) with OSPFv3 on their interconnecting link.  The test ensures OSPFv3
neighbors are established, routes are exchanged between the routers, and
end-to-end IPv6 connectivity is achieved.

An end-device (HOST) is connected to R2 on an interface without OSPFv3 enabled.
This verifies that OSPFv3 status information remains accessible when a router
has non-OSPFv3 interfaces.

Note: OSPFv3 has no IPv4 address to derive a router-id from, so an
explicit-router-id is configured on every router.
"""

# TODO: Remove HOST node once Infamy supports unconnected ports in topologies

import infamy
import infamy.route as route
from infamy.util import until, parallel

OSPFV3 = "infix-routing:ospfv3"


def config_target1(target, data, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": data,
                        "enabled": True,
                        "ipv6": {
                            "forwarding": True,
                            "address": [{
                                "ip": "2001:db8:10::1",
                                "prefix-length": 64
                            }]}
                    },
                    {
                        "name": link,
                        "enabled": True,
                        "ipv6": {
                            "forwarding": True,
                            "address": [{
                                "ip": "2001:db8:50::1",
                                "prefix-length": 64
                            }]
                        }
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:100::1",
                                "prefix-length": 128
                            }]
                        }
                    }
                ]
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
                        "type": OSPFV3,
                        "name": "default",
                        "ospf": {
                            "address-family": "ipv6",
                            "explicit-router-id": "1.1.1.1",
                            "redistribute": {
                                "redistribute": [{
                                    "protocol": "static"
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
                                            "name": link,
                                            "hello-interval": 1,
                                            "dead-interval": 3
                                        }]
                                    },
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
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": OSPFV3,
                        "name": "default",
                        "ospf": {
                            "address-family": "ipv6",
                            "explicit-router-id": "2.2.2.2",
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


def config_host(target, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": link,
                    "enabled": True,
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:60::2",
                            "prefix-length": 64
                        }]
                    }
                }]
            }
        }
    })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        R1 = env.attach("R1", "mgmt")
        R2 = env.attach("R2", "mgmt")
        HOST = env.attach("HOST", "mgmt")

    with test.step("Configure targets"):
        _, R1data = env.ltop.xlate("R1", "data")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R1link = env.ltop.xlate("R1", "link")
        _, R2data = env.ltop.xlate("R2", "data")
        _, HOSTlink = env.ltop.xlate("HOST", "link")

        parallel(config_target1(R1, R1data, R1link),
                 config_target2(R2, R2link, R2data),
                 config_host(HOST, HOSTlink))

    with test.step("Wait for OSPFv3 routes"):
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:200::1/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:100::1/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:33::1/128", proto="ietf-ospf:ospfv3"), attempts=200)

    with test.step("Verify R2 OSPFv3 neighbors with non-OSPFv3 interface"):
        assert route.ospf_has_neighbors(R2, proto=OSPFV3)

    with test.step("Test connectivity from PC:data to 2001:db8:200::1"):
        _, hport0 = env.ltop.xlate("PC", "data")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip("2001:db8:10::2", prefix_length=64, proto="ipv6")
            ns0.addroute("2001:db8:200::1/128", "2001:db8:10::1", proto="ipv6")
            ns0.must_reach("2001:db8:200::1")

    test.succeed()
