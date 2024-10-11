#!/usr/bin/env python3
#
#
#             10.0.0.1/32 (lo)
#                |
#         +------+---------+ .1   10.0.12.0/30         .2+--------------------+
#         |      R1        +-----------------------------+      R2            |
#         |                |       AREA0                 |                    +-10.0.0.2/32 (lo)
#         +-------+--------+--.1                     .1 -+--------+-----------+
#              .2 |           \---                  ---/          |.1
#                 |               \---         ----/              |
#   10.0.41.0 /30 | AREA2             \--- ---/                   | 10.0.23.0/30
#                 |       10.0.24.0/30 ---/\---  10.0.13.0/30     |
#                 |               ----/        \---       AREA1   |
#              .1 |        .2 ---/                  \--- .2       |.2
#         +-------+--------+-/                        \-+--------+----------+
#         |   R4        .2 |                             |   R3              |
#         |                +---------+                 .1|                   +-10.0.0.3/32 (lo)
#         +------+---------+.1       | 192.168.4.0/24    +-------------------+
#                |                   |.2
#          10.0.0.4/32 (lo)  +-------+
#                            |       |
#                            |  PC   |
#                            |       |
#                            +-------+
#
#
"""
OSPF with multiple areas

This test tests a lot of features inside OSPF using 3 areas (one NSSA area, with no summary)
to test the distribution of routes is deterministic (using cost), also test
link breaks using BFD (not implemented in infamy though)

This test also verifies broadcast and point-to-point interface types on /30 network and
explicit router-id.
"""
import infamy

import infamy.route as route
from infamy.util import until, parallel
def config_target1(target, ring1, ring2, cross):
    target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": ring1,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.12.1",
                                "prefix-length": 30
                            }]}
                    },
                    {
                        "name": ring2,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.41.2",
                                "prefix-length": 30
                            }]}
                    },
                    {
                        "name": cross,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.13.1",
                                "prefix-length": 30
                            },
                            {
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
                                    "ip": "11.0.8.1",
                                    "prefix-length": 24
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
                        "explicit-router-id": "10.0.0.1",
                        "areas": {
                            "area": [{
                                "area-id": "0.0.0.0",
                                "interfaces":
                                {
                                    "interface": [{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": ring1,
                                        "hello-interval": 1,
                                        "enabled": True
                                    }]
                                }
                            },{
                            "area-id": "0.0.0.1",
                            "area-type": "nssa-area",
                            "summary": False,
                            "interfaces":
                                {
                                    "interface": [{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": cross,
                                        "hello-interval": 1,
                                        "enabled": True,
                                        "cost": 2000
                                    }]
                                }
                            },{
                                "area-id": "0.0.0.2",
                                "interfaces":
                                {
                                    "interface": [{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": ring2,
                                        "hello-interval": 1,
                                        "enabled": True,
                                        "interface-type": "point-to-point"
                                    },
                                    {
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
    })
    target.put_config_dict("ietf-system", {
            "system": {
                "hostname": "R1"
            }
        })

def config_target2(target, ring1, ring2, cross):
    target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": ring1,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.23.1",
                                "prefix-length": 30
                            }]}
                    },
                    {
                        "name": ring2,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.12.2",
                                "prefix-length": 30
                            }]}
                    },
                    {
                        "name": cross,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.24.1",
                                "prefix-length": 30
                            }]}
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": "10.0.0.2",
                                "prefix-length": 32
                            }, {
                                "ip": "11.0.9.1",
                                "prefix-length": 24
                            }, {
                                "ip": "11.0.10.1",
                                "prefix-length": 24
                            }, {
                                "ip": "11.0.11.1",
                                "prefix-length": 24
                            }, {
                                "ip": "11.0.12.1",
                                "prefix-length": 24
                            }, {
                                "ip": "11.0.13.1",
                                "prefix-length": 24
                            }, {
                                "ip": "11.0.14.1",
                                "prefix-length": 24
                            }, {
                                "ip": "11.0.15.1",
                                "prefix-length": 24
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
                "control-plane-protocol": [{
                    "type": "infix-routing:ospfv2",
                    "name": "default",
                    "ospf": {
                        "explicit-router-id": "1.1.1.1",
                        "areas": {
                            "area": [{
                                "area-id": "0.0.0.0",
                                "interfaces": {
                                    "interface": [{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": ring2,
                                        "hello-interval": 1,
                                        "enabled": True
                                    }, {
                                        "name": "lo",
                                        "enabled": True
                                    }]
                                }
                            }, {
                                "area-id": "0.0.0.1",
                                "area-type": "nssa-area",
                                "summary": False,
                                "interfaces": {
                                    "interface": [{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": ring1,
                                        "hello-interval": 1,
                                    }]
                                }
                            }, {
                                "area-id": "0.0.0.2",
                                "interfaces": {
                                    "interface": [{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": cross,
                                        "hello-interval": 1,
                                        "cost": 2000
                                    }]
                                }
                            }]
                        }
                    }
                }]
            }
        }
    })


def config_target3(target, ring2, cross, link):
    target.put_config_dict("ietf-interfaces", {
        "interfaces": {
            "interface": [
                {
                    "name": ring2,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "10.0.23.2",
                            "prefix-length": 30
                            }]
                        }
                },
                {
                    "name": link,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "192.168.3.1",
                            "prefix-length": 24
                        }]
                    }
                },
                {
                    "name": cross,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": "10.0.13.2",
                            "prefix-length": 30
                        }]
                    }
                },
                {
                    "name": "lo",
                    "enabled": True,
                    "ipv4": {
                        "address": [{
                            "ip": "10.0.0.3",
                            "prefix-length": 32
                        }]
                    }
                    }
                ]
            }
    })

    target.put_config_dict("ietf-system", {
            "system": {
                "hostname": "R3"
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
                                "area-id": "0.0.0.1",
                                "area-type": "nssa-area",
                                "summary": False,
                                "interfaces": {
                                    "interface": [{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": cross,
                                        "hello-interval": 1,
                                        "enabled": True,
                                        "cost": 2000
                                    }, {
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": ring2,
                                        "hello-interval": 1,
                                        "enabled": True
                                    }, {
                                        "name": link,
                                        "enabled": True,
                                        "passive": True
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
    })


def config_target4(target, ring1, cross, link):
    target.put_config_dict("ietf-interfaces", {
        "interfaces": {
                "interface": [
                    {
                        "name": ring1,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.41.1",
                                "prefix-length": 30
                            }]}
                    },
                    {
                        "name": cross,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "10.0.24.2",
                                "prefix-length": 30
                            }]}
                    },
                    {
                        "name": link,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "192.168.4.1",
                                "prefix-length": 24
                            }]
                        }
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": "10.0.0.4",
                                "prefix-length": 32
                            }]
                        }
                    }
                ]
            }
        })

    target.put_config_dict("ietf-system", {
        "system": {
            "hostname": "R4"
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
                            "redistribute": {
                                "redistribute": [{
                                        "protocol": "connected"
                                    }]
                            },
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.2",
                                    "interfaces": {
                                        "interface": [{
                                            "bfd": {
                                                "enabled": True
                                            },
                                            "name": ring1,
                                            "hello-interval": 1,
                                            "enabled": True,
                                            "interface-type": "point-to-point"
                                        }, {
                                            "bfd": {
                                                "enabled": True
                                            },
                                            "name": cross,
                                            "hello-interval": 1,
                                            "enabled": True,
                                            "cost": 5000
                                        }, {
                                            "name": "lo",
                                            "enabled": True
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


def disable_link(target, link):
    target.put_config_dict("ietf-interfaces", {
        "interfaces": {
            "interface": [
                {
                    "name": link,
                    "enabled": False
                }]
        }
    })


with infamy.Test() as test:
    with test.step("Configure targets"):
        env = infamy.Env()
        R1 = env.attach("R1", "mgmt")
        R2 = env.attach("R2", "mgmt")
        R3 = env.attach("R3", "mgmt")
        R4 = env.attach("R4", "mgmt")

        _, R1ring1 = env.ltop.xlate("R1", "ring1")
        _, R1ring2 = env.ltop.xlate("R1", "ring2")
        _, R2ring1 = env.ltop.xlate("R2", "ring1")
        _, R2ring2 = env.ltop.xlate("R2", "ring2")
        _, R3ring2 = env.ltop.xlate("R3", "ring2")
        _, R4ring1 = env.ltop.xlate("R4", "ring1")

        _, R3data = env.ltop.xlate("R3", "data")
        _, R4data = env.ltop.xlate("R4", "data")

        _, R1cross = env.ltop.xlate("R1", "cross")
        _, R2cross = env.ltop.xlate("R2", "cross")
        _, R3cross = env.ltop.xlate("R3", "cross")
        _, R4cross = env.ltop.xlate("R4", "cross")
        parallel(config_target1(R1, R1ring1, R1ring2, R1cross),
                 config_target2(R2, R2ring1, R2ring2, R2cross),
                 config_target3(R3, R3ring2, R3cross, R3data),
                 config_target4(R4, R4ring1, R4cross, R4data))

    with test.step("Wait for all neighbors to peer"):
        print("Waiting for neighbors to peer")
        until(lambda: route.ospf_get_neighbor(R1, "0.0.0.0", R1ring1, "1.1.1.1"), attempts=200)
        until(lambda: route.ospf_get_neighbor(R1, "0.0.0.1", R1cross, "10.0.0.3"), attempts=200)
        until(lambda: route.ospf_get_neighbor(R2, "0.0.0.1", R2ring1, "10.0.0.3"), attempts=200)
        until(lambda: route.ospf_get_neighbor(R2, "0.0.0.0", R2ring2, "10.0.0.1"), attempts=200)
        until(lambda: route.ospf_get_neighbor(R2, "0.0.0.2", R2cross, "10.0.0.4"), attempts=200)

    with test.step("Wait for routes from OSPF on all routers"):
        print("Waiting for routes from OSPF")
        until(lambda: route.ipv4_route_exist(R1, "10.0.0.2/32", nexthop="10.0.12.2", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R1, "10.0.0.3/32", nexthop="10.0.13.2", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R1, "10.0.0.4/32", nexthop="10.0.41.1", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R1, "192.168.4.0/24", nexthop="10.0.41.1", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R1, "10.0.24.0/30", nexthop="10.0.41.1", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "10.0.0.1/32", nexthop="10.0.23.2", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "10.0.0.3/32", nexthop="10.0.23.2", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "10.0.0.4/32", nexthop="10.0.24.2", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R3, "0.0.0.0/0", nexthop="10.0.23.1", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R4, "10.0.0.3/32", nexthop="10.0.41.2", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "10.0.13.0/30", nexthop="10.0.23.2", proto="ietf-ospf:ospfv2"), attempts=200)

    with test.step("Verify Area 0.0.0.1 on R3 is NSSA area"):
        assert(route.ospf_is_area_nssa(R3, "0.0.0.1"))

    with test.step("Verify R1:ring2 is of type point-to-point"):
        assert(route.ospf_get_interface_type(R1, "0.0.0.2", R1ring2) == "point-to-point")

    with test.step("Verify R4:ring1 is of type point-to-point"):
        assert(route.ospf_get_interface_type(R4, "0.0.0.2", R4ring1) == "point-to-point")

    with test.step("Verify on R3, there are no routes beyond 10.0.23.1, just a default route"):
        # Should be only default route out of the area.
        parallel(until(lambda: route.ipv4_route_exist(R3, "0.0.0.0/0"), attempts=200),
                 until(lambda: route.ipv4_route_exist(R3, "10.0.12.0/30") is False, attempts=5),
                 until(lambda: route.ipv4_route_exist(R3, "10.0.12.0/30") is False, attempts=5),
                 until(lambda: route.ipv4_route_exist(R3, "11.0.8.0/24") is False, attempts=5),
                 until(lambda: route.ipv4_route_exist(R3, "11.0.9.0/24") is False, attempts=5),
                 until(lambda: route.ipv4_route_exist(R3, "11.0.10.0/24") is False, attempts=5),
                 until(lambda: route.ipv4_route_exist(R3, "11.0.11.0/24") is False, attempts=5),
                 until(lambda: route.ipv4_route_exist(R3, "11.0.12.0/24") is False, attempts=5),
                 until(lambda: route.ipv4_route_exist(R3, "11.0.13.0/24") is False, attempts=5),
                 until(lambda: route.ipv4_route_exist(R3, "11.0.14.0/24") is False, attempts=5),
                 until(lambda: route.ipv4_route_exist(R3, "11.0.15.0/24") is False, attempts=5))

    _, hport0 = env.ltop.xlate("PC", "data3")
    with infamy.IsolatedMacVlan(hport0) as ns0:
        with test.step("Testing connectivity through NSSA area, from PC:data3 to 11.0.8.1"):
            ns0.addip("192.168.3.2")
            ns0.addroute("0.0.0.0/0", "192.168.3.1")
            ns0.must_reach("11.0.8.1")

    _, hport0 = env.ltop.xlate("PC", "data4")
    with infamy.IsolatedMacVlan(hport0) as ns0:
        ns0.addip("192.168.4.2")
        ns0.addroute("0.0.0.0/0", "192.168.4.1")
        with test.step("Verify that the route to 10.0.0.3 from PC:data4, go through 10.0.41.2"):
            trace = ns0.traceroute("10.0.0.3")
            assert len(trace) == 3
            assert trace[1][1] == "10.0.41.2"
            assert trace[2][1] == "10.0.0.3"

        with test.step("Break link R1:ring2 --- R4:ring1"):
            # Here we should test with link breakers, to test BFD
            # recouppling, for now disable the link
            disable_link(R1, R1ring2)

        with test.step("Verify that the route to 10.0.0.3 from PC:data4, go through 10.0.24.1"):
            until(lambda: route.ipv4_route_exist(R4, "10.0.0.3/32", nexthop="10.0.24.1", proto="ietf-ospf:ospfv2"), attempts=100)
            until(lambda: route.ipv4_route_exist(R4, "10.0.0.3/32", nexthop="10.0.41.2") is False, attempts=10)
            trace = ns0.traceroute("10.0.0.3")
            assert len(trace) == 3
            assert trace[1][1] == "10.0.24.1"
            assert trace[2][1] == "10.0.0.3"
            ns0.must_reach("10.0.0.3")

    test.succeed()
