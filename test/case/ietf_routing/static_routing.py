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

import copy
import infamy
import time
import infamy.iface as iface
import infamy.route as route
from infamy.util import until

def config_target1(target, data, link):
    target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": data,
                        "type": "infix-if-type:ethernet",
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                            "ip": "192.168.10.1",
                            "prefix-length": 24
                            }]
                        }
                    },
                    {
                        "name": link,
                        "type": "infix-if-type:ethernet",
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                            "ip": "192.168.50.1",
                            "prefix-length": 24
                            }]
                        }
                    },
                    {
                        "name": "lo",
                        "type": "infix-if-type:ethernet",
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                            "ip": "192.168.100.1",
                            "prefix-length": 32
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
                        "type": "infix-if-type:ethernet",
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                            "ip": "192.168.50.2",
                            "prefix-length": 24
                            }]
                        }
                    },
                    {
                        "name": "lo",
                        "type": "infix-if-type:ethernet",
                        "enabled": True,
                        "forwarding": True,
                        "ipv4": {
                            "address": [{
                            "ip": "192.168.200.1",
                            "prefix-length": 32
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
                        }
                    }
                }]
            }
        }
    })

def config_remove_routes(target):
    running = target.get_config_dict("/ietf-routing:routing")
    new = copy.deepcopy(running)
    new["routing"]["control-plane-protocols"].clear()
    target.put_diff_dicts("ietf-routing",running,new)
    
with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("2x2"))
        target1 = env.attach("target1", "mgmt")
        target2 = env.attach("target2", "mgmt")

    with test.step("Configure target physical ports [enable routing]"):
        _, target1data = env.ltop.xlate("target1", "data")
        _, target2_to_target1 = env.ltop.xlate("target2", "target1")
        _, target1_to_target2 = env.ltop.xlate("target1", "target2")

        config_target1(target1, target1data, target1_to_target2)
        config_target2(target2, target2_to_target1)
        until(lambda: route.ipv4_route_exist(target1, "192.168.200.1/32"))
        until(lambda: route.ipv4_route_exist(target2, "0.0.0.0/0"))

    with test.step("Wait for links"):
        until(lambda: iface.get_oper_up(target1, target1data))
        until(lambda: iface.get_oper_up(target1, target1data))
        until(lambda: iface.get_oper_up(target1, target1_to_target2))
        until(lambda: iface.get_oper_up(target2, target2_to_target1))

    with test.step("Ping from host to 192.168.200.1(dut2) through dut1"):
        _, hport0 = env.ltop.xlate("host", "data1")
        with infamy.IsolatedMacVlan(hport0) as ns0:
             ns0.addip("192.168.10.2")
             ns0.addroute("192.168.200.1/32", "192.168.10.1")
             ns0.must_reach("192.168.200.1")
    with test.step("Remove static routes on dut1"):
        config_remove_routes(target1);
        until(lambda: route.ipv4_route_exist(target1, "192.168.200.1/32") == False)

    with test.step("Ping from host to 192.168.200.1(dut2) through dut1 (should not be possible)"):
        _, hport0 = env.ltop.xlate("host", "data1")
        with infamy.IsolatedMacVlan(hport0) as ns0:
             ns0.addip("192.168.10.2")
             ns0.addroute("192.168.200.1/32", "192.168.10.1")
             ns0.must_not_reach("192.168.200.1")
    test.succeed()
