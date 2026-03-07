#!/usr/bin/env python3
"""OSPF Point-to-Multipoint

Verify OSPF point-to-multipoint (non-broadcast) interface type by
configuring three routers on a shared multi-access network with the
ietf-ospf 'point-to-multipoint' interface type and static neighbors.
This maps to FRR's 'point-to-multipoint non-broadcast' network type,
which requires manual neighbor configuration since there is no
multicast neighbor discovery.

R2 acts as the hub, bridging two physical links (link1, link2) into a
single broadcast domain (br0).  R1 and R3 each connect to one of R2's
ports.  The test verifies that all routers form OSPF adjacencies via
unicast, exchange routes, and that the interface type is correctly
reported as point-to-multipoint.

....
  +------------------+                                   +------------------+
  |       R1         |                                   |       R3         |
  |  10.0.1.1/32     |                                   |  10.0.3.1/32     |
  |     (lo)         |                                   |     (lo)         |
  +--------+---------+                                   +--------+---------+
           |  .1                                                  |  .3
           |               +------------------+                   |
           +----link1------+       R2         +------link2--------+
                           |  10.0.2.1/32     |
                           |     (lo)         |
                           | br0: 10.0.123.2  |
                           +------------------+
                              10.0.123.0/24
                      (P2MP non-broadcast / shared segment)
....
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel


def config_target1(target, link, data):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": link,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.123.1",
                                "prefix-length": 24
                            }]
                        }
                    },
                    {
                        "name": data,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.10.1",
                                "prefix-length": 24
                            }]
                        }
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": "10.0.1.1",
                                "prefix-length": 32
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
                                            "name": link,
                                            "enabled": True,
                                            "hello-interval": 1,
                                            "dead-interval": 3,
                                            "interface-type": "point-to-multipoint",
                                            "static-neighbors": {
                                                "neighbor": [
                                                    {"identifier": "10.0.123.2"},
                                                    {"identifier": "10.0.123.3"}
                                                ]
                                            }
                                        }, {
                                            "name": "lo",
                                            "enabled": True
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


def config_target2(target, link1, link2):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.123.2",
                                "prefix-length": 24
                            }]
                        }
                    },
                    {
                        "name": link1,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": link2,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": "10.0.2.1",
                                "prefix-length": 32
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
                                            "name": "br0",
                                            "enabled": True,
                                            "hello-interval": 1,
                                            "dead-interval": 3,
                                            "interface-type": "point-to-multipoint",
                                            "static-neighbors": {
                                                "neighbor": [
                                                    {"identifier": "10.0.123.1"},
                                                    {"identifier": "10.0.123.3"}
                                                ]
                                            }
                                        }, {
                                            "name": "lo",
                                            "enabled": True
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


def config_target3(target, link, data):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": link,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.123.3",
                                "prefix-length": 24
                            }]
                        }
                    },
                    {
                        "name": data,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.30.1",
                                "prefix-length": 24
                            }]
                        }
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": "10.0.3.1",
                                "prefix-length": 32
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
                                            "name": link,
                                            "enabled": True,
                                            "hello-interval": 1,
                                            "dead-interval": 3,
                                            "interface-type": "point-to-multipoint",
                                            "static-neighbors": {
                                                "neighbor": [
                                                    {"identifier": "10.0.123.1"},
                                                    {"identifier": "10.0.123.2"}
                                                ]
                                            }
                                        }, {
                                            "name": "lo",
                                            "enabled": True
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


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        R1 = env.attach("R1", "mgmt")
        R2 = env.attach("R2", "mgmt")
        R3 = env.attach("R3", "mgmt")


    with test.step("Configure targets"):
        _, R1link = env.ltop.xlate("R1", "link")
        _, R1data = env.ltop.xlate("R1", "data")
        _, R2link1 = env.ltop.xlate("R2", "link1")
        _, R2link2 = env.ltop.xlate("R2", "link2")
        _, R3link = env.ltop.xlate("R3", "link")
        _, R3data = env.ltop.xlate("R3", "data")

        parallel(config_target1(R1, R1link, R1data),
                 config_target2(R2, R2link1, R2link2),
                 config_target3(R3, R3link, R3data))

    with test.step("Wait for OSPF routes"):
        print("Waiting for OSPF routes from all routers")
        until(lambda: route.ipv4_route_exist(R1, "10.0.2.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R1, "10.0.3.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "10.0.1.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "10.0.3.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R3, "10.0.1.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R3, "10.0.2.1/32", proto="ietf-ospf:ospfv2"), attempts=200)

    with test.step("Verify interface type is point-to-multipoint"):
        print("Checking OSPF interface type on all routers")
        assert route.ospf_get_interface_type(R1, "0.0.0.0", R1link) == "point-to-multipoint"
        assert route.ospf_get_interface_type(R2, "0.0.0.0", "br0") == "point-to-multipoint"
        assert route.ospf_get_interface_type(R3, "0.0.0.0", R3link) == "point-to-multipoint"

    with test.step("Verify connectivity between all DUTs"):
        _, hport1 = env.ltop.xlate("PC", "data1")
        _, hport2 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport1) as ns1, \
             infamy.IsolatedMacVlan(hport2) as ns2:
            ns1.addip("10.0.10.2")
            ns2.addip("10.0.30.2")
            ns1.addroute("10.0.3.1/32", "10.0.10.1")
            ns2.addroute("10.0.1.1/32", "10.0.30.1")
            parallel(
                lambda: ns1.must_reach("10.0.3.1"),
                lambda: ns2.must_reach("10.0.1.1"),
            )
    test.succeed()
