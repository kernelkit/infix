#!/usr/bin/env python3
"""OSPFv3 with multiple areas

Evaluates OSPFv3 across three areas (Area 0, Area 1 = NSSA, Area 2) over IPv6
to ensure deterministic (cost-based) route distribution.  It also verifies
broadcast vs point-to-point interface types on the transit links, explicit
router-id, and BFD-triggered fail-over on a link break.

Differences from the OSPFv2 multi-area test:
 - FRR ospf6d has no totally-NSSA (NSSA no-summary), so the "NSSA area sees
   only a default route" assertion is not mirrored; the area is a regular NSSA.
 - OSPFv3 route next-hops are IPv6 link-local addresses, so the data path is
   verified with traceroute (transit link global addresses) and reachability.
 - OSPFv3 has no IPv4 to derive a router-id from; explicit-router-id is set on
   every router.

....
                 2001:db8::1/32 (lo)
                        R1
        (Area0) 2001:db8:12::/64   R2
        (Area2) 2001:db8:41::/64   |  (Area1) 2001:db8:23::/64
        (Area1) 2001:db8:13::/64   |  (Area2) 2001:db8:24::/64
                        R4         R3
....
"""
import infamy
import infamy.route as route
from infamy.util import until, parallel

OSPFV3 = "infix-routing:ospfv3"


def config_target1(target, ring1, ring2, cross):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {"name": ring1, "enabled": True,
                     "ipv6": {"forwarding": True, "address": [{"ip": "2001:db8:12::1", "prefix-length": 64}]}},
                    {"name": ring2, "enabled": True,
                     "ipv6": {"forwarding": True, "address": [{"ip": "2001:db8:41::2", "prefix-length": 64}]}},
                    {"name": cross, "enabled": True,
                     "ipv6": {"forwarding": True, "address": [{"ip": "2001:db8:13::1", "prefix-length": 64}]}},
                    {"name": "lo", "enabled": True,
                     "ipv6": {"address": [{"ip": "2001:db8::1", "prefix-length": 128}]}}
                ]
            }
        },
        "ietf-system": {"system": {"hostname": "R1"}},
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": OSPFV3,
                        "name": "default",
                        "ospf": {
                            "explicit-router-id": "10.0.0.1",
                            "areas": {
                                "area": [
                                    {"area-id": "0.0.0.0",
                                     "interfaces": {"interface": [
                                         {"bfd": {"enabled": True}, "name": ring1, "hello-interval": 1, "enabled": True}]}},
                                    {"area-id": "0.0.0.1", "area-type": "nssa-area",
                                     "interfaces": {"interface": [
                                         {"bfd": {"enabled": True}, "name": cross, "hello-interval": 1, "enabled": True, "cost": 2000},
                                         {"name": "lo", "enabled": True}]}},
                                    {"area-id": "0.0.0.2",
                                     "interfaces": {"interface": [
                                         {"bfd": {"enabled": True}, "name": ring2, "hello-interval": 1, "enabled": True, "interface-type": "point-to-point"}]}}
                                ]
                            }
                        }
                    }]
                }
            }
        }
    })


def config_target2(target, ring1, ring2, cross):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {"name": ring1, "enabled": True,
                     "ipv6": {"forwarding": True, "address": [{"ip": "2001:db8:23::1", "prefix-length": 64}]}},
                    {"name": ring2, "enabled": True,
                     "ipv6": {"forwarding": True, "address": [{"ip": "2001:db8:12::2", "prefix-length": 64}]}},
                    {"name": cross, "enabled": True,
                     "ipv6": {"forwarding": True, "address": [{"ip": "2001:db8:24::1", "prefix-length": 64}]}},
                    {"name": "lo", "enabled": True,
                     "ipv6": {"address": [{"ip": "2001:db8::2", "prefix-length": 128}]}}
                ]
            }
        },
        "ietf-system": {"system": {"hostname": "R2"}},
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": OSPFV3,
                        "name": "default",
                        "ospf": {
                            "explicit-router-id": "1.1.1.1",
                            "areas": {
                                "area": [
                                    {"area-id": "0.0.0.0",
                                     "interfaces": {"interface": [
                                         {"bfd": {"enabled": True}, "name": ring2, "hello-interval": 1, "enabled": True},
                                         {"name": "lo", "enabled": True}]}},
                                    {"area-id": "0.0.0.1", "area-type": "nssa-area",
                                     "interfaces": {"interface": [
                                         {"bfd": {"enabled": True}, "name": ring1, "hello-interval": 1, "enabled": True}]}},
                                    {"area-id": "0.0.0.2",
                                     "interfaces": {"interface": [
                                         {"bfd": {"enabled": True}, "name": cross, "hello-interval": 1, "cost": 2000, "enabled": True}]}}
                                ]
                            }
                        }
                    }]
                }
            }
        }
    })


def config_target3(target, ring2, cross, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {"name": ring2, "enabled": True,
                     "ipv6": {"forwarding": True, "address": [{"ip": "2001:db8:23::2", "prefix-length": 64}]}},
                    {"name": link, "enabled": True,
                     "ipv6": {"forwarding": True, "address": [{"ip": "2001:db8:3::1", "prefix-length": 64}]}},
                    {"name": cross, "enabled": True,
                     "ipv6": {"forwarding": True, "address": [{"ip": "2001:db8:13::2", "prefix-length": 64}]}},
                    {"name": "lo", "enabled": True,
                     "ipv6": {"address": [{"ip": "2001:db8::3", "prefix-length": 128}]}}
                ]
            }
        },
        "ietf-system": {"system": {"hostname": "R3"}},
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": OSPFV3,
                        "name": "default",
                        "ospf": {
                            "explicit-router-id": "10.0.0.3",
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.1", "area-type": "nssa-area",
                                    "interfaces": {"interface": [
                                        {"bfd": {"enabled": True}, "name": cross, "hello-interval": 1, "enabled": True, "cost": 2000},
                                        {"bfd": {"enabled": True}, "name": ring2, "hello-interval": 1, "enabled": True},
                                        {"name": link, "enabled": True, "passive": True},
                                        {"name": "lo", "enabled": True}]}
                                }]
                            }
                        }
                    }]
                }
            }
        }
    })


def config_target4(target, ring1, cross, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {"name": ring1, "enabled": True,
                     "ipv6": {"forwarding": True, "address": [{"ip": "2001:db8:41::1", "prefix-length": 64}]}},
                    {"name": cross, "enabled": True,
                     "ipv6": {"forwarding": True, "address": [{"ip": "2001:db8:24::2", "prefix-length": 64}]}},
                    {"name": link, "enabled": True,
                     "ipv6": {"forwarding": True, "address": [{"ip": "2001:db8:4::1", "prefix-length": 64}]}},
                    {"name": "lo", "enabled": True,
                     "ipv6": {"address": [{"ip": "2001:db8::4", "prefix-length": 128}]}}
                ]
            }
        },
        "ietf-system": {"system": {"hostname": "R4"}},
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": OSPFV3,
                        "name": "default",
                        "ospf": {
                            "explicit-router-id": "10.0.0.4",
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.2",
                                    "interfaces": {"interface": [
                                        {"bfd": {"enabled": True}, "name": ring1, "hello-interval": 1, "enabled": True, "interface-type": "point-to-point"},
                                        {"bfd": {"enabled": True}, "name": cross, "hello-interval": 1, "enabled": True, "cost": 5000},
                                        # Advertise the PC:data4 network as an intra-area OSPFv3
                                        # prefix (passive), not via redistribute-connected.  ospf6d
                                        # has no totally-NSSA, so area 0.0.0.1 is a regular NSSA:
                                        # it accepts inter-area (Type-3) summaries but blocks
                                        # AS-external (Type-5) LSAs.  A redistributed external here
                                        # would never reach R3, breaking its return path.
                                        {"name": link, "enabled": True, "passive": True},
                                        {"name": "lo", "enabled": True}]}
                                }]
                            }
                        }
                    }]
                }
            }
        }
    })


def disable_link(target, link):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{"name": link, "enabled": False}]
            }
        }
    })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
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

    with test.step("Configure targets"):
        parallel(config_target1(R1, R1ring1, R1ring2, R1cross),
                 config_target2(R2, R2ring1, R2ring2, R2cross),
                 config_target3(R3, R3ring2, R3cross, R3data),
                 config_target4(R4, R4ring1, R4cross, R4data))

    with test.step("Wait for all neighbors to peer"):
        print("Waiting for neighbors to peer")
        until(lambda: route.ospf_get_neighbor(R1, "0.0.0.0", R1ring1, "1.1.1.1", proto=OSPFV3), attempts=200)
        until(lambda: route.ospf_get_neighbor(R1, "0.0.0.1", R1cross, "10.0.0.3", proto=OSPFV3), attempts=200)
        until(lambda: route.ospf_get_neighbor(R2, "0.0.0.1", R2ring1, "10.0.0.3", proto=OSPFV3), attempts=200)
        until(lambda: route.ospf_get_neighbor(R2, "0.0.0.0", R2ring2, "10.0.0.1", proto=OSPFV3), attempts=200)
        until(lambda: route.ospf_get_neighbor(R2, "0.0.0.2", R2cross, "10.0.0.4", proto=OSPFV3), attempts=200)

    with test.step("Wait for cross-area OSPFv3 loopback routes"):
        print("Waiting for routes from OSPFv3")
        until(lambda: route.ipv6_route_exist(R1, "2001:db8::2/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R1, "2001:db8::3/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R1, "2001:db8::4/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R4, "2001:db8::3/128", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R2, "2001:db8::1/128", proto="ietf-ospf:ospfv3"), attempts=200)
        # R3 (regular NSSA) must learn R4's data network as an inter-area
        # summary for its return path towards PC:data4.
        until(lambda: route.ipv6_route_exist(R3, "2001:db8:4::/64", proto="ietf-ospf:ospfv3"), attempts=200)

    with test.step("Verify Area 0.0.0.1 on R3 is NSSA area"):
        assert route.ospf_is_area_nssa(R3, "0.0.0.1", proto=OSPFV3)

    with test.step("Verify R1:ring2 is of type point-to-point"):
        assert route.ospf_get_interface_type(R1, "0.0.0.2", R1ring2, proto=OSPFV3) == "point-to-point"

    with test.step("Verify R4:ring1 is of type point-to-point"):
        assert route.ospf_get_interface_type(R4, "0.0.0.2", R4ring1, proto=OSPFV3) == "point-to-point"

    _, hport0 = env.ltop.xlate("PC", "data4")
    with infamy.IsolatedMacVlan(hport0) as ns0:
        ns0.addip("2001:db8:4::2", prefix_length=64, proto="ipv6")
        ns0.addroute("::/0", "2001:db8:4::1", proto="ipv6")

        with test.step("Verify route to 2001:db8::3 from PC:data4 goes through 2001:db8:41::2 (R1)"):
            ns0.must_reach("2001:db8::3")
            trace = ns0.traceroute("2001:db8::3")
            hops = [row[1] for row in trace]
            assert "2001:db8:41::2" in hops, f"Path to R3 ({repr(hops)}) does not go through R1"

        with test.step("Break link R1:ring2 --- R4:ring1"):
            disable_link(R1, R1ring2)

        with test.step("Verify route to 2001:db8::3 from PC:data4 fails over through 2001:db8:24::1 (R2)"):
            # A plain "route exists" check passes immediately on the stale route
            # still pointing at the now-dead R1 link.  IPv6 next-hops are
            # link-local so we cannot match the new next-hop in the RIB (as the
            # OSPFv2 test does); instead wait until R4's adjacency to R1 is gone,
            # which guarantees SPF has recomputed the path towards R2.
            until(lambda: not route.ospf_get_neighbor(R4, "0.0.0.2", R4ring1, "10.0.0.1", proto=OSPFV3), attempts=200)
            until(lambda: route.ipv6_route_exist(R4, "2001:db8::3/128", proto="ietf-ospf:ospfv3"), attempts=200)
            ns0.must_reach("2001:db8::3")
            trace = ns0.traceroute("2001:db8::3")
            hops = [row[1] for row in trace]
            assert "2001:db8:24::1" in hops, f"Path to R3 ({repr(hops)}) did not fail over via R2"

    test.succeed()
