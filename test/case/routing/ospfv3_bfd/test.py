#!/usr/bin/env python3

"""
OSPFv3 BFD

Verify that a router running OSPFv3, with Bidirectional Forwarding Detection
(BFD) enabled, detects link faults over IPv6 even when the physical layer is
still operational.

This can typically happen when one logical link, from OSPF's perspective, is
made up of multiple physical links containing media converters without link
fault forwarding.

Note: OSPFv3 has no IPv4 address to derive a router-id from, so an
explicit-router-id is configured on every router.  OSPFv3 route next-hops are
IPv6 link-local addresses, so the active path is verified with traceroute
rather than by matching a RIB next-hop.
"""

import time

import infamy
import infamy.route as route
from infamy.netns import TPMR
from infamy.util import until, parallel

OSPFV3 = "infix-routing:ospfv3"


def config(target, params):
    name = params["name"]
    rid = params["rid"]
    dif, fif, sif = \
        params["link"]["data"], \
        params["link"]["fast"], \
        params["link"]["slow"]
    daddr, faddr, saddr = \
        params["addr"]["data"], \
        params["addr"]["fast"], \
        params["addr"]["slow"]

    def ifconfig(name, addr, plen):
        return {
            "name": name,
            "enabled": True,
            "ipv6": {
                "forwarding": True,
                "address": [{
                    "ip": addr,
                    "prefix-length": plen,
                }]}
        }

    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    ifconfig(dif, daddr, 64),
                    ifconfig(fif, faddr, 64),
                    ifconfig(sif, saddr, 64),
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
                            "explicit-router-id": rid,
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.0",
                                    "interfaces": {
                                        # Leave fast/slow as broadcast (the default).  Both are
                                        # parallel links to the same neighbor (R2); with OSPFv3
                                        # point-to-point, ospf6d collapses the link-local next-hop
                                        # and installs only one path, ignoring interface cost.
                                        # Broadcast keeps the two links distinct so cost decides.
                                        "interface": [{
                                            "bfd": {"enabled": True},
                                            "name": fif,
                                            "hello-interval": 1,
                                            "dead-interval": 10,
                                            "cost": 100,
                                        }, {
                                            "bfd": {"enabled": True},
                                            "name": sif,
                                            "hello-interval": 1,
                                            "dead-interval": 10,
                                            "cost": 200,
                                        }, {
                                            "name": dif,
                                            "passive": True,
                                        }]
                                    },
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

    with test.step("Setup TPMR between R1fast and R2fast"):
        breaker = TPMR(env.ltop.xlate("PC", "R1fast")[1],
                       env.ltop.xlate("PC", "R2fast")[1]).start()

    with test.step("Configure R1 and R2"):
        r1cfg = {
            "name": "R1",
            "rid": "192.168.1.1",
            "addr": {
                "data": "2001:db8:10::1",
                "fast": "2001:db8:100::1",
                "slow": "2001:db8:200::1",
            },
            "link": {
                "data": env.ltop.xlate("R1", "h1")[1],
                "fast": env.ltop.xlate("R1", "fast")[1],
                "slow": env.ltop.xlate("R1", "slow")[1],
            }
        }
        r2cfg = {
            "name": "R2",
            "rid": "192.168.1.2",
            "addr": {
                "data": "2001:db8:20::1",
                "fast": "2001:db8:100::2",
                "slow": "2001:db8:200::2",
            },
            "link": {
                "data": env.ltop.xlate("R2", "h2")[1],
                "fast": env.ltop.xlate("R2", "fast")[1],
                "slow": env.ltop.xlate("R2", "slow")[1],
            }
        }

        parallel(config(R1, r1cfg), config(R2, r2cfg))

    with test.step("Setup IP addresses and default routes on h1 and h2"):
        _, h1 = env.ltop.xlate("PC", "h1")
        _, h2 = env.ltop.xlate("PC", "h2")

        h1net = infamy.IsolatedMacVlan(h1).start()
        h1net.addip("2001:db8:10::2", prefix_length=64, proto="ipv6")
        h1net.addroute("default", "2001:db8:10::1", proto="ipv6")

        h2net = infamy.IsolatedMacVlan(h2).start()
        h2net.addip("2001:db8:20::2", prefix_length=64, proto="ipv6")
        h2net.addroute("default", "2001:db8:20::1", proto="ipv6")

    with test.step("Wait for R1 and R2 to peer"):
        print("Waiting for R1 and R2 to peer")
        until(lambda: route.ipv6_route_exist(R1, "2001:db8:20::/64", proto="ietf-ospf:ospfv3"), attempts=200)
        until(lambda: route.ipv6_route_exist(R2, "2001:db8:10::/64", proto="ietf-ospf:ospfv3"), attempts=200)

    with test.step("Verify connectivity from PC:src to PC:dst via fast link"):
        h1net.must_reach("2001:db8:20::2")
        # Fast and slow are broadcast links to the same neighbor.  After the
        # adjacencies reach Full, OSPF still has to finish DR election (its wait
        # timer runs for up to one dead-interval) and originate the Network-LSA
        # before the lower-cost fast link wins in SPF.  A single traceroute can
        # therefore observe the slow path before convergence completes, so poll
        # the data path until it settles on the fast link.
        def via_fast():
            hops = [row[1] for row in h1net.traceroute("2001:db8:20::2")]
            return "2001:db8:100::2" in hops
        until(via_fast, attempts=200)

    with test.step("Disable forwarding between R1fast and R2fast to trigger fail-over"):
        breaker.block()
        print("Give BFD some time to detect the bad link, " +
              "but not enough for the OSPF dead interval expire")
        time.sleep(1)

    with test.step("Verify connectivity from PC:src to PC:dst via slow link"):
        h1net.must_reach("2001:db8:20::2")
        hops = [row[1] for row in h1net.traceroute("2001:db8:20::2")]
        assert "2001:db8:200::2" in hops, f"Path to h2 ({repr(hops)}), does not use slow link"

    test.succeed()
