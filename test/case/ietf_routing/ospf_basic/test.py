#!/usr/bin/env python3
"""
OSPF Basic

Very basic OSPF test just test that OSPF sends HELLO packets between the DUTs
and that they exchange routes, ending with a simple connectivity check.
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
                            }]}
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
                        "redistribute": {
                            "redistribute": [{
                                "protocol": "static"
                            },
                            {
                                "protocol": "connected"
                            }]
                        },
                        "areas": {
                            "area": [{
                                "area-id": "0.0.0.0",
                                "interfaces":
                                {
                                    "interface": [{
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
                        "redistribute": {
                            "redistribute": [{
                                "protocol": "static"
                            },
                            {
                                "protocol": "connected"
                            }]
                        },
                        "areas": {
                            "area": [{
                                "area-id": "0.0.0.0",
                                "interfaces":{
                                    "interface": [{
                                        "name": link,
                                        "hello-interval": 1,
                                        "dead-interval": 3
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
        _, R1link= env.ltop.xlate("R1", "link")

        parallel(config_target1(R1, R1data, R1link),
                 config_target2(R2, R2link))
    with test.step("Wait for OSPF routes"):
        print("Waiting for OSPF routes..")
        until(lambda: route.ipv4_route_exist(R1, "192.168.200.1/32", source_protocol = "infix-routing:ospf"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "192.168.100.1/32", source_protocol = "infix-routing:ospf"), attempts=200)

    with test.step("Test connectivity from PC:data to 192.168.200.1"):
        _, hport0 = env.ltop.xlate("PC", "data")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip("192.168.10.2")
            ns0.addroute("192.168.200.1/32", "192.168.10.1")
            ns0.must_reach("192.168.200.1")

    test.succeed()
