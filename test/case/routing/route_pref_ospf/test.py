#!/usr/bin/env python3
"""
Route preference: {version} vs Static

This test configures a device with both an {version}-acquired route on a
dedicated interface and a static route to the same destination on
another interface. The static route has a higher preference value than
OSPF.

Initially, the device should prefer the OSPF route; if the OSPF route
becomes unavailable, the static route should take over.
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel
from infamy.netns import TPMR


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.add_argument("--version", type=str.lower, choices=["ospfv2", "ospfv3"])


PARAM = {
    "ospfv2": {
        "af":      "ipv4",
        "len":     24,
        "R1": {"data": "192.168.10.1", "link": "192.168.50.1", "ospf": "192.168.60.1"},
        "R2": {"data": "192.168.20.2", "link": "192.168.50.2", "ospf": "192.168.60.2"},
        "dest":    "192.168.20.0/24",
        "default": "0.0.0.0/0",
        "PC1":     "192.168.10.11",
        "PC2":     "192.168.20.22",
    },
    "ospfv3": {
        "af":      "ipv6",
        "len":     64,
        "router-id": {"R1": "1.1.1.1", "R2": "2.2.2.2"},
        "R1": {"data": "2001:db8:10::1", "link": "2001:db8:50::1", "ospf": "2001:db8:60::1"},
        "R2": {"data": "2001:db8:20::2", "link": "2001:db8:50::2", "ospf": "2001:db8:60::2"},
        "dest":    "2001:db8:20::/64",
        "default": "::/0",
        "PC1":     "2001:db8:10::11",
        "PC2":     "2001:db8:20::22",
    },
}


def configure_interface(p, name, ip, forwarding=True):
    return {
        "name": name,
        "enabled": True,
        p["af"]: {
            "forwarding": forwarding,
            "address": [{"ip": ip, "prefix-length": p["len"]}]
        }
    }


def ospf_instance(p, name, ospf):
    conf = {}
    if "router-id" in p:
        conf["explicit-router-id"] = p["router-id"][name]

    conf |= {
        "redistribute": {
            "redistribute": [{"protocol": "connected"}]
        },
        "areas": {
            "area": [{
                "area-id": "0.0.0.0",
                "interfaces": {
                    "interface": [{
                        "name": ospf,
                        "hello-interval": 1,
                        "dead-interval": 3
                    }]
                }
            }]
        }
    }

    return {
        "type": f"infix-routing:{p['version']}",
        "name": "ospf-default",
        "ospf": conf
    }


def config_target1(target, data, link, ospf, p):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    configure_interface(p, data, p["R1"]["data"]),
                    configure_interface(p, link, p["R1"]["link"]),
                    configure_interface(p, ospf, p["R1"]["ospf"])
                ]
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [
                        ospf_instance(p, "R1", ospf),
                        {
                            "type": "infix-routing:static",
                            "name": "dot20",
                            "static-routes": {
                                p["af"]: {
                                    "route": [{
                                        "destination-prefix": p["dest"],
                                        "next-hop": {"next-hop-address": p["R2"]["link"]},
                                        "route-preference": 120
                                    }]
                                }
                            }
                        }
                    ]
                }
            }
        }
    })


def config_target2(target, data, link, ospf, p):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    configure_interface(p, data, p["R2"]["data"]),
                    configure_interface(p, link, p["R2"]["link"]),
                    configure_interface(p, ospf, p["R2"]["ospf"])
                ]
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [
                        ospf_instance(p, "R2", ospf),
                        {
                            "type": "infix-routing:static",
                            "name": "default",
                            "static-routes": {
                                p["af"]: {
                                    "route": [{
                                        "destination-prefix": p["default"],
                                        "next-hop": {"next-hop-address": p["R1"]["link"]}
                                    }]
                                }
                            }
                        }
                    ]
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

        R1, R2 = parallel(lambda: env.attach("R1", "mgmt"),
                          lambda: env.attach("R2", "mgmt"))

    with test.step("Set up TPMR between R1ospf and R2ospf"):
        ospf_breaker = TPMR(env.ltop.xlate("PC", "R1_ospf")[1], env.ltop.xlate("PC", "R2_ospf")[1]).start()

    with test.step("Configure targets"):
        _, R1data = env.ltop.xlate("R1", "data")
        _, R1link = env.ltop.xlate("R1", "link")
        _, R1ospf = env.ltop.xlate("R1", "ospf")
        _, R2data = env.ltop.xlate("R2", "data")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R2ospf = env.ltop.xlate("R2", "ospf")

        parallel(lambda: config_target1(R1, R1data, R1link, R1ospf, param),
                 lambda: config_target2(R2, R2data, R2link, R2ospf, param))

    with test.step("Set up persistent MacVlan namespaces"):
        _, hport_data1 = env.ltop.xlate("PC", "data1")
        _, hport_data2 = env.ltop.xlate("PC", "data2")

        ns1 = infamy.IsolatedMacVlan(hport_data1).start()
        ns1.addip(param["PC1"], prefix_length=param["len"], proto=af)
        ns1.addroute("default", param["R1"]["data"], proto=af)

        ns2 = infamy.IsolatedMacVlan(hport_data2).start()
        ns2.addip(param["PC2"], prefix_length=param["len"], proto=af)
        ns2.addroute("default", param["R2"]["data"], proto=af)

    with test.step(f"Wait for {version} and static routes"):
        print("Waiting for OSPF and static routes...")
        until(lambda: route.route_exist(R1, param["dest"], af=af, proto=proto), attempts=200)
        until(lambda: route.route_exist(R1, param["dest"], af=af, proto="ietf-routing:static"), attempts=200)

    with test.step(f"Verify connectivity from PC:data1 to PC:data2 via {version}"):
        ns1.must_reach(param["PC2"])

        ospf_route_active = route.route_exist(R1, param["dest"], af=af, proto=proto, active_check=True)
        assert ospf_route_active, "OSPF route should be preferred when available."

        hops = [row[1] for row in ns1.traceroute(param["PC2"])]
        assert param["R2"]["ospf"] in hops, f"Path does not use expected OSPF route: {hops}"

    with test.step("Simulate OSPF route loss by blocking OSPF interface"):
        ospf_breaker.block()
        until(lambda: not route.route_exist(R1, param["dest"], af=af, proto=proto), attempts=200)

    with test.step("Verify connectivity via static route after OSPF failover"):
        ns1.must_reach(param["PC2"])

        static_route_active = route.route_exist(R1, param["dest"], af=af,
                                                proto="ietf-routing:static", active_check=True)
        assert static_route_active, "Static route should be preferred when OSPF route is unavailable."

        hops = [row[1] for row in ns1.traceroute(param["PC2"])]
        assert param["R2"]["link"] in hops, f"Path does not use expected static route: {hops}"

    test.succeed()
