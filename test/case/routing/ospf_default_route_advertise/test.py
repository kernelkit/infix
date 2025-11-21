#!/usr/bin/env python3
"""
OSPF Default route advertise

Verify _default-route-advertising_ in OSPF, sometimes called 'redistribute origin'. Verify both
always (regardless if a local default route exist or not) and only redistribute
when local default route exist.

The test is performed by setting a default route on R1 to 192.168.10.2, and setting
_default-route-advertising_ in OSPF, this will result that R2 will see a default route.

When set interface R1:data down, OSPF will no longer redistribute default route to R2,
unless _always_ is set for _default-route-advertising_.
....
 +-------------------+      Area 0            +------------------+
 |       R1          |.1  192.168.50.0/24   .2|      R2          |
 | 192.169.100.1/32  +------------------------+  192.168.200.1/32|
 | 10.10.10.10/32    |R1:link         R2:link |                  |
 +--------------+----+                        +---+--------------+
        R1:data |.1                       R2:data |.1
                |                                 |
                | 192.168.10.0/24                 | 192.168.20.0/24
                |                                 |
    host:data1  |.2                    host:data2 |.2
          +-----+---------------------------------+-------+
          |                                               |
          |             host                              |
          |                                               |
          +-----------------------------------------------+
....
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel


def config_target1(target, data, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": "dummy0",
                        "enabled": True,
                        "type": "infix-if-type:dummy",
                        "ipv4": {
                            "address": [{
                                "ip": "10.10.10.10",
                                "prefix-length": 32
                            }]
                        }
                    },
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
    },
    "ietf-routing": {
        "routing": {
            "control-plane-protocols": {
                "control-plane-protocol": [
                    {
                        "type": "infix-routing:static",
                        "name": "default",
                        "static-routes": {
                            "ipv4": {
                                "route": [
                                {
                                    "destination-prefix": "0.0.0.0/0",
                                    "next-hop": {
                                        "next-hop-address": "192.168.10.2"
                                    }
                                }]
                            }
                        }
                    },
                    {
                        "type": "infix-routing:ospfv2",
                        "name": "default",
                        "ospf": {
                            "explicit-router-id": "1.1.1.1",
                            "default-route-advertise": {
                                "enabled": True
                            },
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.0",
                                    "interfaces":
                                    {
                                        "interface": [{
                                            "name": link,
                                            "enabled": True,
                                            "hello-interval": 1,
                                            "dead-interval": 3
                                        },
                                        {
                                            "name": "lo",
                                            "enabled": True
                                        }]
                                    },
                                }]
                            }
                        }
                    }
                ]
            }
        }
    }
})


def config_target2(target, data, link):
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
                                "ip": "192.168.50.2",
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
                                "ip": "192.168.20.1",
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
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [
                    {
                            "type": "infix-routing:ospfv2",
                            "name": "default",
                            "ospf": {
                                "explicit-router-id": "2.2.2.2",
                                "areas": {
                                    "area": [{
                                        "area-id": "0.0.0.0",
                                        "interfaces":{
                                            "interface": [{
                                                "enabled": True,
                                                "name": link,
                                                "hello-interval": 1,
                                                "dead-interval": 3
                                            },
                                            {
                                                "name": data,
                                                "passive": True,
                                                "enabled": True
                                            },
                                            {
                                                "enabled": True,
                                                "name": "lo"
                                            }]
                                        }
                                    }]
                                }
                            }
                    }
                ]
            }
        }
    }
})


def disable_interface(target, iface):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": iface,
                        "enabled": False,
                    }
                    ]
            }
        }
    })


def set_redistribute_default_always(target):
    target.put_config_dicts({
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [
                    {
                        "type": "infix-routing:ospfv2",
                        "name": "default",
                        "ospf": {
                            "default-route-advertise": {
                                "enabled": True,
                                "always": True
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

    with test.step("Configure targets"):
        _, R1data = env.ltop.xlate("R1", "data")
        _, R2data = env.ltop.xlate("R2", "data")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R1link= env.ltop.xlate("R1", "link")

        parallel(config_target1(R1, R1data, R1link),
                 config_target2(R2, R2data, R2link))
    with test.step("Verify R2 has a default route and 192.168.100.1/32 from OSPF"):
        print("Waiting for OSPF routes...")
        until(lambda: route.ipv4_route_exist(R2, "0.0.0.0/0", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "192.168.100.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R1, "192.168.200.1/32", proto="ietf-ospf:ospfv2"), attempts=200)

    with test.step("Verify connectivity from PC:data2 to 10.10.10.10"):
        _, hport0 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip("192.168.20.2")
            ns0.addroute("0.0.0.0/0", "192.168.20.1")
            #breakpoint()
            ns0.must_reach("10.10.10.10")

    with test.step("Disable link PC:data1 <--> R1:data (take default gateway down)"):
        disable_interface(R1, R1data)

    with test.step("Verify R2 does not have a default route but a 192.168.100.1/32 from OSPF"):
        until(lambda: route.ipv4_route_exist(R2, "192.168.100.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R1, "192.168.200.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "0.0.0.0/0", proto="ietf-ospf:ospfv2") == False, attempts=200)

    with test.step("Verify no connectivity from PC:data2 to 10.10.10.10"):
        _, hport0 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip("192.168.20.2")
            ns0.addroute("0.0.0.0/0", "192.168.20.1")
            ns0.must_not_reach("10.10.10.10")

    with test.step("Enable redistribute default route 'always' on R1"):
        set_redistribute_default_always(R1)

    with test.step("Wait for all neighbors to peer"):
        until(lambda: route.ospf_get_neighbor(R1, "0.0.0.0", R1link, "2.2.2.2"), attempts=200)
        until(lambda: route.ospf_get_neighbor(R2, "0.0.0.0", R2link, "1.1.1.1"), attempts=200)

    with test.step("Verify R2 has a default route and 192.168.100.1/32 from OSPF"):
        print("Waiting for OSPF routes...")
        until(lambda: route.ipv4_route_exist(R2, "192.168.100.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R1, "192.168.200.1/32", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "0.0.0.0/0", proto="ietf-ospf:ospfv2"), attempts=200)

    with test.step("Verify connectivity from PC:data2 to 10.10.10.10"):
        _, hport0 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip("192.168.20.2")
            ns0.addroute("0.0.0.0/0", "192.168.20.1")
            ns0.must_reach("10.10.10.10")

    test.succeed()
