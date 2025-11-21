#!/usr/bin/env python3

"""
OSPF BFD

Verify that a router running OSPF, with Bidirectional Forwarding
Detection (BFD) enabled, will detect link faults even when the
physical layer is still operational.

This can typically happen when one logical link, from OSPF's
perspective, is made up of multiple physical links containing media
converters without link fault forwarding.
"""

import time

import infamy
import infamy.route as route
from infamy.netns import TPMR
from infamy.util import until, parallel

def config(target, params):
    name = params["name"]
    dif, fif, sif = \
        params["link"]["data"], \
        params["link"]["fast"], \
        params["link"]["slow"]
    rid, daddr, faddr, saddr = \
        params["addr"]["rid"], \
        params["addr"]["data"], \
        params["addr"]["fast"], \
        params["addr"]["slow"]

    def ifconfig(name, addr, plen):
        return {
            "name": name,
            "enabled": True,
            "ipv4": {
                "forwarding": True,
                "address": [{
                    "ip": addr,
                    "prefix-length": plen,
                }]}
        }

    target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    ifconfig("lo", rid, 32),

                    ifconfig(dif, daddr, 24),
                    ifconfig(fif, faddr, 30),
                    ifconfig(sif, saddr, 30),
                ]
            }
    })

    target.put_config_dict("ietf-system", {
        "system": {
            "hostname": name,
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
                                "area-id": "0.0.0.0",
                                "interfaces":
                                {
                                    "interface": [{
                                        "bfd": {
                                            "enabled": True
                                        },
                                        "name": fif,
                                        "hello-interval": 1,
                                        "dead-interval": 10,
                                        "cost": 100,
                                    },
                                    {
                                        "bfd": {
                                            "enabled": True
                                        },
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
            "addr": {
                "rid": "192.168.1.1",

                "data": "192.168.10.1",
                "fast": "192.168.100.1",
                "slow": "192.168.200.1",
            },
            "link": {
                "data": env.ltop.xlate("R1", "h1")[1],
                "fast": env.ltop.xlate("R1", "fast")[1],
                "slow": env.ltop.xlate("R1", "slow")[1],
            }
        }
        r2cfg = {
            "name": "R2",
            "addr": {
                "rid": "192.168.1.2",

                "data": "192.168.20.1",
                "fast": "192.168.100.2",
                "slow": "192.168.200.2",
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
        h1net.addip("192.168.10.2")
        h1net.addroute("default", "192.168.10.1")

        h2net = infamy.IsolatedMacVlan(h2).start()
        h2net.addip("192.168.20.2")
        h2net.addroute("default", "192.168.20.1")

    with test.step("Wait for R1 and R2 to peer"):
        print("Waiting for R1 and R2 to peer")
        until(lambda: route.ipv4_route_exist(R1, "192.168.20.0/24", "192.168.100.2", proto="ietf-ospf:ospfv2"), attempts=200)
        until(lambda: route.ipv4_route_exist(R2, "192.168.10.0/24", "192.168.100.1", proto="ietf-ospf:ospfv2"), attempts=200)

    with test.step("Verify connectivity from PC:src to PC:dst via fast link"):
        h1net.must_reach("192.168.20.2")
        hops = [row[1] for row in h1net.traceroute("192.168.20.2")]
        assert "192.168.100.2" in hops, f"Path to h2 ({repr(hops)}), does not use fast link"

    with test.step("Disable forwarding between R1fast and R2fast to trigger fail-over"):
        breaker.block()
        print("Give BFD some time to detect the bad link, " +
              "but not enough for the OSPF dead interval expire")
        time.sleep(1)

    with test.step("Verify connectivity from PC:src to PC:dst via slow link"):
        h1net.must_reach("192.168.20.2")
        hops = [row[1] for row in h1net.traceroute("192.168.20.2")]
        assert "192.168.200.2" in hops, f"Path to h2 ({repr(hops)}), does not use slow link"

    test.succeed()
