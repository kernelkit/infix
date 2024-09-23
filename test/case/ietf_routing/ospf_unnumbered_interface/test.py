#!/usr/bin/env python3
"""
OSPF unnumbered interfaces

This test that a configuration expecting unnumbered interfaces
get that also in OSPF. Also verify that passive interface in
the configuration gets activated in OSPF.

When this test pass, you can expect unnumbered interfaces, interface type
configuration and passive to function
"""

import infamy
import time

import infamy.route as route
from infamy.util import until, parallel
# This test tests passive interfaces and unnumbered interfaces.

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
                            }]}
                    },
                    {
                        "name": link,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.0.1",
                                "prefix-length": 32
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
                        }
                    }
                ]
            }
    })

    target.put_config_dict("ietf-routing", {
        "routing": {
            "control-plane-protocols": {
                "control-plane-protocol": [{
                    "type": "infix-routing:ospfv2",
                    "name": "default",
                    "ospf": {
                        "areas": {
                            "area": [{
                                "area-id": "0.0.0.0",
                                "interfaces":
                                {
                                    "interface": [{
                                        "name": link,
                                        "hello-interval": 1,
                                        "dead-interval": 3,
                                        "interface-type": "point-to-point"
                                    }, {
                                        "name": data,
                                        "passive": True
                                    }, {
                                        "name": "lo",
                                        "passive": True
                                    }]
                                },
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
                                "ip": "10.0.0.2",
                                "prefix-length": 32
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
                        }
                    }
                ]
            }
        })

    target.put_config_dict("ietf-routing", {
        "routing": {
            "control-plane-protocols": {
                "control-plane-protocol": [
                    {
                        "type": "infix-routing:ospfv2",
                        "name": "default",
                        "ospf": {
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.0",
                                    "interfaces": {
                                        "interface": [{
                                            "name": link,
                                            "hello-interval": 1,
                                            "dead-interval": 3,
                                            "interface-type": "point-to-point"
                                        }, {
                                            "name": "lo",
                                            "passive": True
                                        }]
                                    }
                                }]
                            }
                        }
                    }
                ]
            }
        }
    })


with infamy.Test() as test:
    with test.step("Configure targets"):
        env = infamy.Env()
        R1 = env.attach("R1", "mgmt")
        R2 = env.attach("R2", "mgmt")

        _, R1data = env.ltop.xlate("R1", "data")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R1link = env.ltop.xlate("R1", "link")

        parallel(lambda: config_target1(R1, R1data, R1link),
                 lambda: config_target2(R2, R2link))
    with test.step("Wait for OSPF routes"):
        print("Waiting for OSPF routes..")
        until(lambda: route.ipv4_route_exist(R1, "192.168.200.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "192.168.100.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "192.168.10.0/24", proto="ietf-ospf:ospfv2"), attempts=200)

    with test.step("Check interface type"):
        assert(route.ospf_get_interface_type(R1, "0.0.0.0", R1link) == "point-to-point")
        assert(route.ospf_get_interface_type(R2, "0.0.0.0", R2link) == "point-to-point")

        _, hport0 = env.ltop.xlate("PC", "data")
    with infamy.IsolatedMacVlan(hport0) as ns0:
        ns0.addip("192.168.10.2")
        ns0.addroute("192.168.200.1/32", "192.168.10.1")

        with test.step("Verify there are no OSPF HELLO packets on PC:data"):
            assert(route.ospf_get_interface_passive(R1, "0.0.0.0", R1data))
            print("Verify that no hello packets are recieved from passive interfaces")
            ns0.must_not_receive("ip proto 89", timeout=15) # Default hello time 10s

        with test.step("Test connectivity from PC:data to 192.168.200.1"):
            ns0.must_reach("192.168.200.1")

    test.succeed()
