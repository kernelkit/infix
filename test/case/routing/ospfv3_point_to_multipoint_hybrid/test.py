#!/usr/bin/env python3
"""OSPFv3 Point-to-Multipoint Hybrid

Verify OSPFv3 point-to-multipoint hybrid interface type by configuring three
routers on a shared multi-access IPv6 network with the ietf-ospf 'hybrid'
interface type.  This maps to FRR ospf6d's 'point-to-multipoint' network type,
which uses multicast for neighbor discovery.

R2 acts as the hub, bridging two physical links (link1, link2) into a single
broadcast domain (br0).  R1 and R3 each connect to one of R2's ports.  The test
verifies that all routers form OSPFv3 adjacencies, exchange routes, and that
the interface type is reported as hybrid.

Note: OSPFv3 has no IPv4 address to derive a router-id from, so an
explicit-router-id is configured on every router.
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel

OSPFV3 = "infix-routing:ospfv3"


def config_target1(target, link, data):
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
                                "ip": "2001:db8:123::1",
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
                                "ip": "2001:db8:10::1",
                                "prefix-length": 64
                            }]
                        }
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:1::1",
                                "prefix-length": 128
                            }]
                        }
                    }
                ]
            }
        },
        "ietf-system": {
            "system": {"hostname": "R1"}
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": OSPFV3,
                        "name": "default",
                        "ospf": {
                            "explicit-router-id": "1.1.1.1",
                            "redistribute": {
                                "redistribute": [{"protocol": "connected"}]
                            },
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.0",
                                    "interfaces": {
                                        "interface": [{
                                            "name": link,
                                            "enabled": True,
                                            "interface-type": "hybrid",
                                            "hello-interval": 1,
                                            "dead-interval": 3
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
                        "ipv6": {
                            "forwarding": True,
                            "address": [{
                                "ip": "2001:db8:123::2",
                                "prefix-length": 64
                            }]
                        }
                    },
                    {
                        "name": link1,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {"bridge": "br0"}
                    },
                    {
                        "name": link2,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {"bridge": "br0"}
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:2::1",
                                "prefix-length": 128
                            }]
                        }
                    }
                ]
            }
        },
        "ietf-system": {
            "system": {"hostname": "R2"}
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": OSPFV3,
                        "name": "default",
                        "ospf": {
                            "explicit-router-id": "2.2.2.2",
                            "redistribute": {
                                "redistribute": [{"protocol": "connected"}]
                            },
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.0",
                                    "interfaces": {
                                        "interface": [{
                                            "name": "br0",
                                            "enabled": True,
                                            "interface-type": "hybrid",
                                            "hello-interval": 1,
                                            "dead-interval": 3
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
                        "ipv6": {
                            "forwarding": True,
                            "address": [{
                                "ip": "2001:db8:123::3",
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
                                "ip": "2001:db8:30::1",
                                "prefix-length": 64
                            }]
                        }
                    },
                    {
                        "name": "lo",
                        "enabled": True,
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:3::1",
                                "prefix-length": 128
                            }]
                        }
                    }
                ]
            }
        },
        "ietf-system": {
            "system": {"hostname": "R3"}
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": OSPFV3,
                        "name": "default",
                        "ospf": {
                            "explicit-router-id": "3.3.3.3",
                            "redistribute": {
                                "redistribute": [{"protocol": "connected"}]
                            },
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.0",
                                    "interfaces": {
                                        "interface": [{
                                            "name": link,
                                            "enabled": True,
                                            "interface-type": "hybrid",
                                            "hello-interval": 1,
                                            "dead-interval": 3
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

    with test.step("Wait for OSPFv3 routes"):
        print("Waiting for OSPFv3 routes to converge")
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:2::1/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:3::1/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:1::1/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:3::1/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R3, "2001:db8:1::1/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R3, "2001:db8:2::1/128", proto="ietf-ospf:ospfv3"), attempts=200)

    with test.step("Verify interface type is hybrid"):
        print("Checking OSPFv3 interface type on all routers")
        assert route.ospf_get_interface_type(R1, "0.0.0.0", R1link, proto=OSPFV3) == "hybrid"
        assert route.ospf_get_interface_type(R2, "0.0.0.0", "br0", proto=OSPFV3) == "hybrid"
        assert route.ospf_get_interface_type(R3, "0.0.0.0", R3link, proto=OSPFV3) == "hybrid"

    with test.step("Verify connectivity between all DUTs"):
        _, hport1 = env.ltop.xlate("PC", "data1")
        _, hport2 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport1) as ns1, \
             infamy.IsolatedMacVlan(hport2) as ns2:
            ns1.addip("2001:db8:10::2", prefix_length=64, proto="ipv6")
            ns2.addip("2001:db8:30::2", prefix_length=64, proto="ipv6")
            ns1.addroute("2001:db8:3::1/128", "2001:db8:10::1", proto="ipv6")
            ns2.addroute("2001:db8:1::1/128", "2001:db8:30::1", proto="ipv6")
            parallel(
                lambda: ns1.must_reach("2001:db8:3::1"),
                lambda: ns2.must_reach("2001:db8:1::1"),
            )
    test.succeed()
