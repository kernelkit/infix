#!/usr/bin/env python3
#  192.168.100.1/32 (lo)
#        |
#   +--------------+                +-------------+
#   |              |192.168.50.0/24 |             |
#   |    R1     .1 +----------------+ .2    R2    |-192.168.200.1/32 (lo)
#   |        .1    |                |  .1         |
#   +--+------+----+                +---+-----+---+
#      |      |                               |
# MGMT |      | <-192.168.10.0/24             |MGMT
#      |      |                               |
#      |      |                               |
#    +-+------+-------------------------+-----+--+
#    |       .2                                  |
#    |                                           |
#    |                                           |
#    +-------------------------------------------+
"""
Static routing

Verify that it is possible to add static routes, both IPv4 and IPv6, and
that data forwarding works as expected via an intermediate device.
"""
import infamy
import infamy.route as route
from infamy.util import until, parallel


def config_target1(target, data, link):
    target.put_config_dict("ietf-interfaces", {
        "interfaces": {
            "interface": [
                {
                    "name": data,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.10.1",
                            "prefix-length": 24
                        }]},
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:3c4d:10::1",
                            "prefix-length": 64
                        }]
                    }
                },
                {
                    "name": link,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.50.1",
                            "prefix-length": 24
                        }]
                    },
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": "2001:db8:3c4d:50::1",
                            "prefix-length": 64
                        }]
                    }
                },
                {
                    "name": "lo",
                    "enabled": True,
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.100.1",
                            "prefix-length": 32
                        }]
                    },
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:3c4d:100::1",
                            "prefix-length": 128
                        }]
                    }

                    }
            ]
        }
    })
    target.put_config_dict("ietf-routing", {
        "routing": {
            "control-plane-protocols": {
                "control-plane-protocol": [{
                    "type": "infix-routing:static",
                    "name": "default",
                    "static-routes": {
                        "ipv4": {
                            "route": [{
                                "destination-prefix": "192.168.200.1/32",
                                "next-hop": {
                                    "next-hop-address": "192.168.50.2"
                                }
                            }]
                        },
                        "ipv6": {
                            "route": [{
                                "destination-prefix": "2001:db8:3c4d:200::1/128",
                                "next-hop": {
                                    "next-hop-address": "2001:db8:3c4d:50::2"
                                }
                            }]
                        }
                    }
                }]
            }
        }
    })


def config_target2(target, link):
    target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": link,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "192.168.50.2",
                                "prefix-length": 24
                            }]
                        },
                        "ipv6": {
                            "forwarding": True,
                            "address": [{
                                "ip": "2001:db8:3c4d:50::2",
                                "prefix-length": 64
                            }]
                        }
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "forwarding": True,
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.200.1",
                                "prefix-length": 32
                            }]
                        },
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:3c4d:200::1",
                                "prefix-length": 128
                            }]
                        }

                    }
                ]
            }
        })

    target.put_config_dict("ietf-routing", {
        "routing": {
            "control-plane-protocols": {
                "control-plane-protocol": [{
                    "type": "infix-routing:static",
                    "name": "default",
                    "static-routes": {
                        "ipv4": {
                            "route": [{
                                "destination-prefix": "0.0.0.0/0",
                                "next-hop": {
                                    "next-hop-address": "192.168.50.1"
                                }
                            }]
                        },
                        "ipv6": {
                            "route": [{
                                "destination-prefix": "::/0",
                                "next-hop": {
                                    "next-hop-address": "2001:db8:3c4d:50::1"
                                }
                            }]
                        }
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

        _, R1data = env.ltop.xlate("R1", "data")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R1link = env.ltop.xlate("R1", "link")

    with test.step("Configure targets"):
        parallel(config_target1(R1, R1data, R1link),
                 config_target2(R2, R2link))

    with test.step("Wait for routes"):
        until(lambda: route.ipv4_route_exist(R1, "192.168.200.1/32"))
        until(lambda: route.ipv4_route_exist(R2, "0.0.0.0/0"))
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:3c4d:200::1/128"))
        until(lambda: route.ipv6_route_exist(R2, "::/0"))

    _, hport0 = env.ltop.xlate("PC", "data")
    with infamy.IsolatedMacVlan(hport0) as ns0:
        with test.step("Configure host addresses and routes"):
            ns0.addip("2001:db8:3c4d:10::2", prefix_length=64, proto="ipv6")
            ns0.addroute("2001:db8:3c4d:200::1/128", "2001:db8:3c4d:10::1", proto="ipv6")
            ns0.addip("192.168.10.2")
            ns0.addroute("192.168.200.1/32", "192.168.10.1")

        with test.step("Verify that R2 is reachable on 192.168.200.1 from PC:data"):
            ns0.must_reach("192.168.200.1")

        with test.step("Verify that R2 is reachable on 2001:db8:3c4d:200::1 from PC:data"):
            ns0.must_reach("2001:db8:3c4d:200::1")

        with test.step("Remove all static routes from R1"):
            R1.delete_xpath("/ietf-routing:routing/control-plane-protocols")
            parallel(until(lambda: route.ipv4_route_exist(R1, "192.168.200.1/32") is False),
                     until(lambda: route.ipv6_route_exist(R1, "2001:db8:3c4d:200::1/128") is False))

        with test.step("Verify R2 is no longer reachable on either IPv4 or IPv6 from PC:data"):
            infamy.parallel(ns0.must_not_reach("192.168.200.1"),
                            ns0.must_not_reach("2001:db8:3c4d:200::1"))

    test.succeed()
