#!/usr/bin/env python3

"""
{version} BFD

Verify that a router running {version}, with Bidirectional Forwarding
Detection (BFD) enabled, will detect link faults even when the
physical layer is still operational.

This can typically happen when one logical link, from OSPF's
perspective, is made up of multiple physical links containing media
converters without link fault forwarding.

Note: OSPFv3 next-hops are IPv6 link-local addresses, so the active path is
verified with traceroute rather than by matching a RIB next-hop, and its BFD
peers are only known by their session count.
"""

import time

import infamy
import infamy.route as route
from infamy.netns import TPMR
from infamy.util import until, parallel


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.add_argument("--version", type=str.lower, choices=["ospfv2", "ospfv3"])


PARAM = {
    "ospfv2": {
        "af":   "ipv4",
        "len":  {"data": 24, "link": 30},
        # Generous OSPF dead interval, so that a fail-over completing well
        # within it can only have been triggered by BFD.
        "dead": 60,
        "R1":   {"rid": "192.168.1.1", "data": "192.168.10.1",
                 "fast": "192.168.100.1", "slow": "192.168.200.1"},
        "R2":   {"rid": "192.168.1.2", "data": "192.168.20.1",
                 "fast": "192.168.100.2", "slow": "192.168.200.2"},
        "h1":   "192.168.10.2",
        "h2":   "192.168.20.2",
        "h1net": "192.168.10.0/24",
        "h2net": "192.168.20.0/24",
    },
    "ospfv3": {
        "af":   "ipv6",
        "len":  {"data": 64, "link": 64},
        # ospf6d only starts DR election once its wait timer, one dead
        # interval, has expired, so keep the dead interval down.
        "dead": 10,
        "R1":   {"rid": "192.168.1.1", "data": "2001:db8:10::1",
                 "fast": "2001:db8:100::1", "slow": "2001:db8:200::1"},
        "R2":   {"rid": "192.168.1.2", "data": "2001:db8:20::1",
                 "fast": "2001:db8:100::2", "slow": "2001:db8:200::2"},
        "h1":   "2001:db8:10::2",
        "h2":   "2001:db8:20::2",
        "h1net": "2001:db8:10::/64",
        "h2net": "2001:db8:20::/64",
    },
}


def config(target, name, links, p):
    """Configure one router, fast link cheaper than the slow link"""
    addr = p[name]

    def ifconfig(ifname, address, plen):
        return {
            "name": ifname,
            "enabled": True,
            p["af"]: {
                "forwarding": True,
                "address": [{
                    "ip": address,
                    "prefix-length": plen,
                }]}
        }

    def ospf_interface(ifname, cost):
        return {
            "bfd": {
                "enabled": True
            },
            "name": ifname,
            "hello-interval": 1,
            "dead-interval": p["dead"],
            "cost": cost,
        }

    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    ifconfig(links["data"], addr["data"], p["len"]["data"]),
                    ifconfig(links["fast"], addr["fast"], p["len"]["link"]),
                    ifconfig(links["slow"], addr["slow"], p["len"]["link"]),
                ]
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": f"infix-routing:{p['version']}",
                        "name": "default",
                        "ospf": {
                            "explicit-router-id": addr["rid"],
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.0",
                                    # Leave fast/slow as broadcast (the default).  Both are
                                    # parallel links to the same neighbor; with OSPFv3
                                    # point-to-point, ospf6d collapses the link-local next-hop
                                    # and installs only one path, ignoring interface cost.
                                    # Broadcast keeps the two links distinct so cost decides.
                                    "interfaces": {
                                        "interface": [
                                            ospf_interface(links["fast"], 100),
                                            ospf_interface(links["slow"], 200),
                                            {
                                                "name": links["data"],
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
        env = infamy.Env(args=ArgumentParser())
        version = env.args.version
        param = PARAM[version] | {"version": version}
        af, proto = param["af"], f"ietf-ospf:{version}"
        instance = f"infix-routing:{version}"

        # OSPFv3 installs IPv6 link-local next-hops, which cannot be matched
        # against the configured addresses.
        nexthop = (lambda addr: addr) if af == "ipv4" else (lambda addr: None)

        R1, R2 = parallel(lambda: env.attach("R1", "mgmt"),
                          lambda: env.attach("R2", "mgmt"))

    with test.step("Setup TPMR between R1fast and R2fast"):
        breaker = TPMR(env.ltop.xlate("PC", "R1fast")[1],
                       env.ltop.xlate("PC", "R2fast")[1]).start()

    with test.step("Configure R1 and R2"):
        r1links = {
            "data": env.ltop.xlate("R1", "h1")[1],
            "fast": env.ltop.xlate("R1", "fast")[1],
            "slow": env.ltop.xlate("R1", "slow")[1],
        }
        r2links = {
            "data": env.ltop.xlate("R2", "h2")[1],
            "fast": env.ltop.xlate("R2", "fast")[1],
            "slow": env.ltop.xlate("R2", "slow")[1],
        }

        parallel(lambda: config(R1, "R1", r1links, param),
                 lambda: config(R2, "R2", r2links, param))

    with test.step("Setup IP addresses and default routes on h1 and h2"):
        _, h1 = env.ltop.xlate("PC", "h1")
        _, h2 = env.ltop.xlate("PC", "h2")

        h1net = infamy.IsolatedMacVlan(h1).start()
        h1net.addip(param["h1"], prefix_length=param["len"]["data"], proto=af)
        h1net.addroute("default", param["R1"]["data"], proto=af)

        h2net = infamy.IsolatedMacVlan(h2).start()
        h2net.addip(param["h2"], prefix_length=param["len"]["data"], proto=af)
        h2net.addroute("default", param["R2"]["data"], proto=af)

    with test.step("Wait for R1 and R2 to peer"):
        print("Waiting for R1 and R2 to peer")
        until(lambda: route.route_exist(R1, param["h2net"], af=af, proto=proto,
                                        nexthop=nexthop(param["R2"]["fast"])), attempts=200)
        until(lambda: route.route_exist(R2, param["h1net"], af=af, proto=proto,
                                        nexthop=nexthop(param["R1"]["fast"])), attempts=200)

    with test.step("Wait for BFD sessions on the fast and slow links to come up"):
        print("Waiting for BFD sessions to come up")
        if af == "ipv4":
            for peer in (param["R2"]["fast"], param["R2"]["slow"]):
                until(lambda peer=peer: route.bfd_session_up(R1, peer), attempts=60)
            for peer in (param["R1"]["fast"], param["R1"]["slow"]):
                until(lambda peer=peer: route.bfd_session_up(R2, peer), attempts=60)
        else:
            # OSPFv3 peers over link-local addresses, unknown in advance.
            until(lambda: route.bfd_sessions_up(R1) >= 2, attempts=60)
            until(lambda: route.bfd_sessions_up(R2) >= 2, attempts=60)

    def path_via(link):
        """Does the data path from h1 to h2 use R2's fast or slow link?"""
        hops = [row[1] for row in h1net.traceroute(param["h2"])]
        return param["R2"][link] in hops

    def failed_over():
        """Has R1 given up on the fast link?

        Polled from the control plane rather than with traceroute, which takes
        seconds per attempt on a black-holed path and would drown out the
        fail-over time being measured."""
        if af == "ipv4":
            return route.route_exist(R1, param["h2net"], af=af, proto=proto,
                                     nexthop=param["R2"]["slow"])

        # OSPFv3 installs link-local next-hops, so watch the adjacency instead.
        # It only drops this long before the dead interval when BFD declares
        # the fast link down, and SPF reruns as soon as it does.
        return not route.ospf_get_neighbor(R1, "0.0.0.0", r1links["fast"],
                                           param["R2"]["rid"], proto=instance)

    with test.step("Verify connectivity from PC:src to PC:dst via fast link"):
        h1net.must_reach(param["h2"])
        # After the adjacencies reach Full, OSPF still has to finish DR election
        # and originate the Network-LSA before the lower-cost fast link wins in
        # SPF.  A single traceroute can therefore observe the slow path before
        # convergence completes, so poll until the data path settles.
        until(lambda: path_via("fast"), attempts=200)

    with test.step("Disable forwarding between R1fast and R2fast to trigger fail-over"):
        breaker.block()

    with test.step("Wait for OSPF to fail over to the slow link, once the fast link is no longer qualified by BFD"):
        print("Waiting for fail-over to the slow link")
        start = time.monotonic()
        until(failed_over, attempts=30)
        took = time.monotonic() - start
        assert took < param["dead"] / 2, \
            f"Fail-over took {took:.1f}s, too slow for BFD; " \
            f"OSPF dead interval ({param['dead']}s) suspected"

    with test.step("Verify connectivity from PC:src to PC:dst via slow link"):
        h1net.must_reach(param["h2"])
        until(lambda: path_via("slow"), attempts=30)

    test.succeed()
