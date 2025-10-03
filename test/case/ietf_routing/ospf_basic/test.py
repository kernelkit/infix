#!/usr/bin/env python3
"""OSPF Basic

Verifies basic OSPF functionality by configuring two routers (R1 and R2)
with OSPF on their interconnecting link.  The test ensures OSPF
neighbors are established, routes are exchanged between the routers, and
end-to-end connectivity is achieved.

An end-device (HOST) is connected to R2 on an interface without OSPF enabled.
This verifies that OSPF status information remains accessible when a router
has non-OSPF interfaces.

"""

# TODO: Remove HOST node once Infamy supports unconnected ports in topologies

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
                    "type": "infix-routing:ospfv2",
                    "name": "default",
                    "ospf": {
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
    })


def config_target2(target, link, data):
    target.put_config_dict("ietf-interfaces", {
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
    })

    target.put_config_dict("ietf-system", {
        "system": {
            "hostname": "R2"
        }
    })
    target.put_config_dict("ietf-routing", {
        "routing": {
            "control-plane-protocols": {
                "control-plane-protocol": [{
                    "type": "infix-routing:ospfv2",
                    "name": "default",
                    "ospf": {
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
    })


def config_host(target, link):
    target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [{
                    "name": link,
                    "enabled": True,
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.60.2",
                            "prefix-length": 24
                        }]
                    }
                }]
            }
        })

    target.put_config_dict("ietf-system", {
        "system": {
            "hostname": "HOST"
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
    with test.step("Wait for OSPF routes"):
        until(lambda: route.ipv4_route_exist(R1, "192.168.200.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "192.168.100.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "192.168.33.1/32", proto="ietf-ospf:ospfv2"), attempts=200)

    with test.step("Verify R2 OSPF neighbors with non-OSPF interface"):
        # Regression test for #1169
        assert route.ospf_has_neighbors(R2)

    with test.step("Test connectivity from PC:data to 192.168.200.1"):
        _, hport0 = env.ltop.xlate("PC", "data")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip("192.168.10.2")
            ns0.addroute("192.168.200.1/32", "192.168.10.1")
            ns0.must_reach("192.168.200.1")

    test.succeed()
