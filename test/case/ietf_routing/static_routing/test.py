#!/usr/bin/env python3
#  192.168.100.1/32 (lo)
#        |
#   +--------------+                +-------------+
#   |              |192.168.50.0/24 |             |
#   |  DUT1     .1 +----------------+ .2  DUT2    |-192.168.200.1/32 (lo)
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

Test that it is possible to use static routes (IPv4 and IPv6)
works as expected
"""
import copy
import infamy
import time
import infamy.iface as iface
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
                    "type": "static",
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
                    "type": "static",
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
    with test.step("Initialize"):
        env = infamy.Env()
        target1 = env.attach("target1", "mgmt")
        target2 = env.attach("target2", "mgmt")

    with test.step("Configure targets"):
        _, target1data = env.ltop.xlate("target1", "data")
        _, target2_to_target1 = env.ltop.xlate("target2", "target1")
        _, target1_to_target2 = env.ltop.xlate("target1", "target2")

        parallel(config_target1(target1, target1data, target1_to_target2),
                 config_target2(target2, target2_to_target1))

    with test.step("Wait for routes"):
        until(lambda: route.ipv4_route_exist(target1, "192.168.200.1/32"))
        until(lambda: route.ipv4_route_exist(target2, "0.0.0.0/0"))
        until(lambda: route.ipv6_route_exist(target1, "2001:db8:3c4d:200::1/128"))
        until(lambda: route.ipv6_route_exist(target2, "::/0"))

    _, hport0 = env.ltop.xlate("host", "data1")
    with infamy.IsolatedMacVlan(hport0) as ns0:
        with test.step("Configure host addresses and routes"):
             ns0.addip("2001:db8:3c4d:10::2", prefix_length=64, proto="ipv6")
             ns0.addroute("2001:db8:3c4d:200::1/128", "2001:db8:3c4d:10::1", proto="ipv6")
             ns0.addip("192.168.10.2")
             ns0.addroute("192.168.200.1/32", "192.168.10.1")

        with test.step("Verify that dut2 is reachable from host over IPv4"):
             ns0.must_reach("192.168.200.1")

        with test.step("Verify that dut2 is reachable from host over IPv6"):
             ns0.must_reach("2001:db8:3c4d:200::1")

        with test.step("Remove static routes on dut1"):
            target1.delete_xpath("/ietf-routing:routing/control-plane-protocols")
            parallel(until(lambda: route.ipv4_route_exist(target1, "192.168.200.1/32") == False),
                     until(lambda: route.ipv6_route_exist(target1, "2001:db8:3c4d:200::1/128") == False))

        with test.step("Verify that dut2 is no longer reachable"):
            infamy.parallel(ns0.must_not_reach("192.168.200.1"),
                            ns0.must_not_reach("2001:db8:3c4d:200::1"))

    test.succeed()
