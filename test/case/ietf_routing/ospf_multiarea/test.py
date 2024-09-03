#!/usr/bin/env python3
#
#
#             10.0.0.1/32 (lo)
#                |
#         +------+---------+ .1   10.0.12.1/30         .2+--------------------+
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

This test test alot of features inside OSPF using 3 areas (one NSSA area)
to test the distribution of routes is deterministic (using cost), also test
link breaks using BFD (not implemented in infamy though)
"""
import infamy

import infamy.route as route
from infamy.util import until, parallel
def config_target1(target, ring1, ring2, cross, link):
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
                        "name": link,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                            "ip": "192.168.1.1",
                            "prefix-length": 24
                            }, {
                            "ip": "11.0.8.1",
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
                            "ip": "10.0.13.1",
                            "prefix-length": 30
                            }]
                        }
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                            "ip": "10.0.0.1",
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
                    "type": "ietf-ospf:ospfv2",
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
                                    },{
                                        "name": link,
                                        "passive": True,
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
                                    },{
                                        "name": "lo",
                                        "enabled": True
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
                                        "enabled": True
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

def config_target2(target, ring1, ring2, cross, link):
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
                        "name": link,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                            "ip": "192.168.2.1",
                            "prefix-length": 24
                            },{
                            "ip": "11.0.9.1",
                            "prefix-length": 24
                            },{
                            "ip": "11.0.10.1",
                            "prefix-length": 24
                                },{
                            "ip": "11.0.11.1",
                            "prefix-length": 24
                                },{
                            "ip": "11.0.12.1",
                            "prefix-length": 24
                                },{
                            "ip": "11.0.13.1",
                            "prefix-length": 24
                                },{
                            "ip": "11.0.14.1",
                            "prefix-length": 24
                                },{
                            "ip": "11.0.15.1",
                            "prefix-length": 24
                                }]
                        }
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                            "ip": "10.0.0.2",
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
                "control-plane-protocol": [{
                    "type": "ietf-ospf:ospfv2",
                    "name": "default",
                    "ospf": {
                        "explicit-router-id": "10.0.0.2",
                        "areas": {
                            "area": [{
                                "area-id": "0.0.0.0",
                                "interfaces":{
                                    "interface": [{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": ring2,
                                        "hello-interval": 1,
                                        "enabled": True
                                    },{
                                        "name": "lo",
                                        "enabled": True
                                    },{
                                        "name": link,
                                        "enabled": True,
                                        "passive": True
                                    }]
                                }
                            },
                            {
                                "area-id": "0.0.0.1",
                                "area-type": "nssa-area",
                                "summary": False,
                                "interfaces":{
                                    "interface": [{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": ring1,
                                        "hello-interval": 1,
                                    },{
                                        "name": "lo",
                                        "enabled": True
                                    }]
                                }
                            },
                            {
                                "area-id": "0.0.0.2",
                                "interfaces":{
                                    "interface": [{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": cross,
                                        "hello-interval": 1,
                                        "cost": 2000
                                    },{
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

def config_target3(target, ring1, ring2, cross, link):
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
                    "type": "ietf-ospf:ospfv2",
                    "name": "default",
                    "ospf": {
                        "areas": {
                            "area": [{
                                "area-id": "0.0.0.1",
                                "area-type": "nssa-area",
                                "summary": False,
                                "interfaces":{
                                    "interface": [{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": cross,
                                        "hello-interval": 1,
                                        "enabled": True,
                                        "cost": 2000
                                    },{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": ring2,
                                        "hello-interval": 1,
                                        "enabled": True
                                    },{
                                        "name": link,
                                        "enabled": True,
                                        "passive": True
                                    },{
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

def config_target4(target, ring1, ring2, cross, link):
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
                    "type": "ietf-ospf:ospfv2",
                    "name": "default",
                    "ospf": {
                        "redistribute": {
                            "redistribute": [
                            {
                                "protocol": "connected"
                            }]
                        },
                        "areas": {
                            "area": [{
                                "area-id": "0.0.0.2",
                                "interfaces":{
                                    "interface": [{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": ring1,
                                        "hello-interval": 1,
                                        "enabled": True
                                    },{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": cross,
                                        "hello-interval": 1,
                                        "enabled": True,
                                        "cost": 5000
                                    },{
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
    with test.step("Initialize"):
        env = infamy.Env()
        dut1 = env.attach("dut1", "mgmt")
        dut2 = env.attach("dut2", "mgmt")
        dut3 = env.attach("dut3", "mgmt")
        dut4 = env.attach("dut4", "mgmt")

    with test.step("Configure targets"):
        _, dut1ring1 = env.ltop.xlate("dut1", "ring1")
        _, dut1ring2 = env.ltop.xlate("dut1", "ring2")
        _, dut2ring1 = env.ltop.xlate("dut2", "ring1")
        _, dut2ring2 = env.ltop.xlate("dut2", "ring2")
        _, dut3ring1 = env.ltop.xlate("dut3", "ring1")
        _, dut3ring2 = env.ltop.xlate("dut3", "ring2")
        _, dut4ring1 = env.ltop.xlate("dut4", "ring1")
        _, dut4ring2 = env.ltop.xlate("dut4", "ring2")
        _, dut1data  = env.ltop.xlate("dut1", "data")
        _, dut2data  = env.ltop.xlate("dut2", "data")
        _, dut3data  = env.ltop.xlate("dut3", "data")
        _, dut4data  = env.ltop.xlate("dut4", "data")

        _, dut1cross  = env.ltop.xlate("dut1", "cross")
        _, dut2cross  = env.ltop.xlate("dut2", "cross")
        _, dut3cross  = env.ltop.xlate("dut3", "cross")
        _, dut4cross  = env.ltop.xlate("dut4", "cross")
        parallel(config_target1(dut1, dut1ring1, dut1ring2, dut1cross, dut1data),
                 config_target2(dut2, dut2ring1, dut2ring2, dut2cross, dut2data),
                 config_target3(dut3, dut3ring1, dut3ring2, dut3cross, dut3data),
                 config_target4(dut4, dut4ring1, dut4ring2, dut4cross, dut4data))
    with test.step("Wait for neighbors"):
        print("Waiting for neighbors to peer")
        until(lambda: route.ospf_get_neighbor(dut1, "0.0.0.0", dut1ring1, "10.0.0.2"), attempts=200)
        until(lambda: route.ospf_get_neighbor(dut1, "0.0.0.1", dut1cross, "10.0.0.3"), attempts=200)
        until(lambda: route.ospf_get_neighbor(dut2, "0.0.0.1", dut2ring1, "10.0.0.3"), attempts=200)
        until(lambda: route.ospf_get_neighbor(dut2, "0.0.0.0", dut2ring2, "10.0.0.1"), attempts=200)
        until(lambda: route.ospf_get_neighbor(dut2, "0.0.0.2", dut2cross, "10.0.0.4"), attempts=200)

    with test.step("Wait for routes from OSPF"):
        print("Waiting for routes from OSPF")
        until(lambda: route.ipv4_route_exist(dut1, "10.0.0.2/32", nexthop="10.0.12.2", source_protocol = "infix-routing:ospf"), attempts=200)
        until(lambda: route.ipv4_route_exist(dut1, "10.0.0.3/32", nexthop="10.0.13.2", source_protocol = "infix-routing:ospf"), attempts=200)
        until(lambda: route.ipv4_route_exist(dut1, "10.0.0.4/32", nexthop="10.0.41.1", source_protocol = "infix-routing:ospf"), attempts=200)
        until(lambda: route.ipv4_route_exist(dut1, "192.168.4.0/24", nexthop="10.0.41.1", source_protocol = "infix-routing:ospf"), attempts=200)
        until(lambda: route.ipv4_route_exist(dut1, "10.0.24.0/30", nexthop="10.0.41.1", source_protocol = "infix-routing:ospf"), attempts=200)
        until(lambda: route.ipv4_route_exist(dut2, "10.0.0.1/32", nexthop="10.0.23.2", source_protocol = "infix-routing:ospf"), attempts=200)
        until(lambda: route.ipv4_route_exist(dut2, "10.0.0.3/32", nexthop="10.0.23.2", source_protocol = "infix-routing:ospf"), attempts=200)
        until(lambda: route.ipv4_route_exist(dut2, "10.0.0.4/32", nexthop="10.0.24.2", source_protocol = "infix-routing:ospf"), attempts=200)
        until(lambda: route.ipv4_route_exist(dut3, "0.0.0.0/0", nexthop="10.0.23.1", source_protocol = "infix-routing:ospf"), attempts=200)
        until(lambda: route.ipv4_route_exist(dut4, "10.0.0.3/32", nexthop="10.0.41.2", source_protocol = "infix-routing:ospf"), attempts=200)
        until(lambda: route.ipv4_route_exist(dut2, "10.0.13.0/30", nexthop="10.0.23.2", source_protocol = "infix-routing:ospf"), attempts=200)

    with test.step("Verify NSSA area"): # Should be only default route out of the area.
       parallel(until(lambda: route.ipv4_route_exist(dut4, "11.0.8.0/24"), attempts=200),
                until(lambda: route.ipv4_route_exist(dut4, "11.0.9.0/24"), attempts=200),
                until(lambda: route.ipv4_route_exist(dut4, "11.0.10.0/24"), attempts=200),
                until(lambda: route.ipv4_route_exist(dut4, "11.0.11.0/24"), attempts=200),
                until(lambda: route.ipv4_route_exist(dut4, "11.0.12.0/24"), attempts=200),
                until(lambda: route.ipv4_route_exist(dut3, "0.0.0.0/0"), attempts=200),
                until(lambda: route.ipv4_route_exist(dut3, "10.0.12.0/30") == False, attempts=5),
                until(lambda: route.ipv4_route_exist(dut3, "10.0.12.0/30") == False, attempts=5),
                until(lambda: route.ipv4_route_exist(dut3, "11.0.8.0/24") == False, attempts=5),
                until(lambda: route.ipv4_route_exist(dut3, "11.0.9.0/24") == False, attempts=5),
                until(lambda: route.ipv4_route_exist(dut3, "11.0.10.0/24") == False, attempts=5),
                until(lambda: route.ipv4_route_exist(dut3, "11.0.11.0/24") == False, attempts=5),
                until(lambda: route.ipv4_route_exist(dut3, "11.0.12.0/24") == False, attempts=5),
                until(lambda: route.ipv4_route_exist(dut3, "11.0.13.0/24") == False, attempts=5),
                until(lambda: route.ipv4_route_exist(dut3, "11.0.14.0/24") == False, attempts=5),
                until(lambda: route.ipv4_route_exist(dut3, "11.0.15.0/24") == False, attempts=5))
       assert(route.ospf_is_area_nssa(dut3, "0.0.0.1"))

    _, hport0 = env.ltop.xlate("host", "data3")
    with infamy.IsolatedMacVlan(hport0) as ns0:
        with test.step("Testing connectivitiy through NSSA area"):
            ns0.addip("192.168.3.2")
            ns0.addroute("0.0.0.0/0", "192.168.3.1")
            ns0.must_reach("11.0.8.1")
    _, hport0 = env.ltop.xlate("host", "data4")
    with infamy.IsolatedMacVlan(hport0) as ns0:
        ns0.addip("192.168.4.2")
        ns0.addroute("0.0.0.0/0", "192.168.4.1")
        with test.step("Verify correct hops"):
            trace=ns0.traceroute("10.0.0.3")
            assert(len(trace) == 3)
            assert(trace[1][1] == "10.0.41.2")
            assert(trace[2][1] == "10.0.0.3")

        with test.step("Disable link between R1 and R4, and verify correct hops"):
            disable_link(dut1, dut1ring2) # Here we should test with link breakers, to test BFD recouppling, for now disable the link
            until(lambda: route.ipv4_route_exist(dut4, "10.0.0.3/32", nexthop="10.0.24.1", source_protocol = "infix-routing:ospf"), attempts=100)
            until(lambda: route.ipv4_route_exist(dut4, "10.0.0.3/32", nexthop="10.0.41.2") == False, attempts = 10)
            trace=ns0.traceroute("10.0.0.3")
            assert(len(trace) == 3)
            assert(trace[1][1] == "10.0.24.1")
            assert(trace[2][1] == "10.0.0.3")
            ns0.must_reach("10.0.0.3")

    test.succeed()
