#!/usr/bin/env python3
"""OSPFv3 Default route advertise

Verify _default-route-advertising_ in OSPFv3, sometimes called 'redistribute
origin'.  Verify both 'always' (regardless of whether a local default route
exists) and the conditional mode (only redistribute when a local default route
exists).

R1 has a default route (::/0) via its data interface and enables
default-route-advertise, so R2 learns a default route via OSPFv3.  When R1:data
is taken down the local default is withdrawn and R2 loses the default route,
unless 'always' is set.

Note: OSPFv3 has no IPv4 address to derive a router-id from, so an
explicit-router-id is configured on every router.
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel

OSPFV3 = "infix-routing:ospfv3"


def config_target1(target, data, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": "dummy0",
                        "enabled": True,
                        "type": "infix-if-type:dummy",
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:cafe::10",
                                "prefix-length": 128
                            }]
                        }
                    },
                    {
                        "name": data,
                        "enabled": True,
                        "ipv6": {
                            "forwarding": True,
                            "address": [{
                                "ip": "2001:db8:10::1",
                                "prefix-length": 64
                            }]}
                    },
                    {
                        "name": link,
                        "enabled": True,
                        "ipv6": {
                            "forwarding": True,
                            "address": [{
                                "ip": "2001:db8:50::1",
                                "prefix-length": 64
                            }]
                        }
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:100::1",
                                "prefix-length": 128
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
                                "ipv6": {
                                    "route": [{
                                        "destination-prefix": "::/0",
                                        "next-hop": {
                                            "next-hop-address": "2001:db8:10::2"
                                        }
                                    }]
                                }
                            }
                        },
                        {
                            "type": OSPFV3,
                            "name": "default",
                            "ospf": {
                                "address-family": "ipv6",
                                "explicit-router-id": "1.1.1.1",
                                "default-route-advertise": {
                                    "enabled": True
                                },
                                "areas": {
                                    "area": [{
                                        "area-id": "0.0.0.0",
                                        "interfaces": {
                                            "interface": [{
                                                "name": link,
                                                "enabled": True,
                                                "hello-interval": 1,
                                                "dead-interval": 3
                                            }, {
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
                        "ipv6": {
                            "forwarding": True,
                            "address": [{
                                "ip": "2001:db8:50::2",
                                "prefix-length": 64
                            }]
                        }
                    },
                    {
                        "name": data,
                        "enabled": True,
                        "ipv6": {
                            "forwarding": True,
                            "address": [{
                                "ip": "2001:db8:20::1",
                                "prefix-length": 64
                            }]
                        }
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:200::1",
                                "prefix-length": 128
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
                        "type": OSPFV3,
                        "name": "default",
                        "ospf": {
                            "address-family": "ipv6",
                            "explicit-router-id": "2.2.2.2",
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.0",
                                    "interfaces": {
                                        "interface": [{
                                            "enabled": True,
                                            "name": link,
                                            "hello-interval": 1,
                                            "dead-interval": 3
                                        }, {
                                            "name": data,
                                            "passive": True,
                                            "enabled": True
                                        }, {
                                            "enabled": True,
                                            "name": "lo"
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


def disable_interface(target, iface):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": iface,
                    "enabled": False,
                }]
            }
        }
    })


def set_redistribute_default_always(target):
    target.put_config_dicts({
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": OSPFV3,
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
        _, R1link = env.ltop.xlate("R1", "link")

        parallel(config_target1(R1, R1data, R1link),
                 config_target2(R2, R2data, R2link))

    with test.step("Verify R2 has a default route and 2001:db8:100::1/128 from OSPFv3"):
        print("Waiting for OSPFv3 routes...")
        until(lambda: route.ipv6_route_exist(R2, "::/0", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:100::1/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:200::1/128", proto="ietf-ospf:ospfv3"), attempts=200)

    with test.step("Verify connectivity from PC:data2 to 2001:db8:cafe::10"):
        _, hport0 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip("2001:db8:20::2", prefix_length=64, proto="ipv6")
            ns0.addroute("::/0", "2001:db8:20::1", proto="ipv6")
            ns0.must_reach("2001:db8:cafe::10")

    with test.step("Disable link PC:data1 <--> R1:data (take default gateway down)"):
        disable_interface(R1, R1data)

    with test.step("Verify R2 loses the default route but keeps 2001:db8:100::1/128 from OSPFv3"):
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:100::1/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:200::1/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R2, "::/0", proto="ietf-ospf:ospfv3") == False, attempts=200)

    with test.step("Verify no connectivity from PC:data2 to 2001:db8:cafe::10"):
        _, hport0 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip("2001:db8:20::2", prefix_length=64, proto="ipv6")
            ns0.addroute("::/0", "2001:db8:20::1", proto="ipv6")
            ns0.must_not_reach("2001:db8:cafe::10")

    with test.step("Enable redistribute default route 'always' on R1"):
        set_redistribute_default_always(R1)

    with test.step("Wait for all neighbors to peer"):
        until(lambda: route.ospf_get_neighbor(R1, "0.0.0.0", R1link, "2.2.2.2", proto=OSPFV3), attempts=200)
        until(lambda: route.ospf_get_neighbor(R2, "0.0.0.0", R2link, "1.1.1.1", proto=OSPFV3), attempts=200)

    with test.step("Verify R2 has a default route and 2001:db8:100::1/128 from OSPFv3"):
        print("Waiting for OSPFv3 routes...")
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:100::1/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:200::1/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R2, "::/0", proto="ietf-ospf:ospfv3"), attempts=200)

    with test.step("Verify connectivity from PC:data2 to 2001:db8:cafe::10"):
        _, hport0 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip("2001:db8:20::2", prefix_length=64, proto="ipv6")
            ns0.addroute("::/0", "2001:db8:20::1", proto="ipv6")
            ns0.must_reach("2001:db8:cafe::10")

    test.succeed()
