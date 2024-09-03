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
    target.put_config_dict("ietf-system", {
        "system": {
            "hostname": "R1"
        }
    })


    target.put_config_dict("ietf-routing", {
        "routing": {
            "control-plane-protocols": {
                "control-plane-protocol": [{
                    "type": "ietf-ospf:ospfv2",
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
                                    },
                                    {
                                        "name": data,
                                        "passive": True
                                    },
                                    {
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

    target.put_config_dict("ietf-system", {
        "system": {
            "hostname": "R2"
        }
    })

    target.put_config_dict("ietf-routing", {
        "routing": {
            "control-plane-protocols": {
                "control-plane-protocol": [
                    {
                    "type": "ietf-ospf:ospfv2",
                    "name": "default",
                    "ospf": {
                        "areas": {
                            "area": [{
                                "area-id": "0.0.0.0",
                                "interfaces":{
                                    "interface": [{
                                        "name": link,
                                        "hello-interval": 1,
                                        "dead-interval": 3,
                                        "interface-type": "point-to-point"
                                    },
                                    {
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
    with test.step("Initialize"):
        env = infamy.Env()
        target1 = env.attach("target1", "mgmt")
        target2 = env.attach("target2", "mgmt")

    with test.step("Configure targets"):
        _, target1data = env.ltop.xlate("target1", "data")
        _, target2_to_target1 = env.ltop.xlate("target2", "target1")
        _, target1_to_target2 = env.ltop.xlate("target1", "target2")

        parallel(lambda: config_target1(target1, target1data, target1_to_target2),
                 lambda: config_target2(target2, target2_to_target1))
    with test.step("Wait for OSPF routes"):
        print("Waiting for OSPF routes..")
        until(lambda: route.ipv4_route_exist(target1, "192.168.200.1/32", source_protocol = "infix-routing:ospf"), attempts=200)
        until(lambda: route.ipv4_route_exist(target2, "192.168.100.1/32", source_protocol = "infix-routing:ospf"), attempts=200)
        until(lambda: route.ipv4_route_exist(target2, "192.168.10.0/24", source_protocol = "infix-routing:ospf"), attempts=200)

    with test.step("Check interface type"):
        assert(route.ospf_get_interface_type(target1, "0.0.0.0", target1_to_target2) == "point-to-point")
        assert(route.ospf_get_interface_type(target2, "0.0.0.0", target2_to_target1) == "point-to-point")

        _, hport0 = env.ltop.xlate("host", "data1")
    with infamy.IsolatedMacVlan(hport0) as ns0:
        ns0.addip("192.168.10.2")
        ns0.addroute("192.168.200.1/32", "192.168.10.1")

        with test.step("Test passive interface"):
            assert(route.ospf_get_interface_passive(target1, "0.0.0.0", target1data))
            print("Verify that no hello packets are recieved from passive interfaces")
            ns0.must_not_receive("ip proto 89", timeout=15) # Default hello time 10s

        with test.step("Test connectivity"):
            ns0.must_reach("192.168.200.1")

    test.succeed()
