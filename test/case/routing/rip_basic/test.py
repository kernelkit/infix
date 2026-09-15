#!/usr/bin/env python3
"""{version} Basic

Verifies basic {version} functionality by configuring two routers (R1 and R2)
with {version} on their interconnecting link.  The test ensures routes are
exchanged between the routers and end-to-end connectivity is achieved.

The test PC uses data1 interface to connect to R1's data port, and data2
interface to connect to R2's data port (which does not have {version} enabled).
This verifies that RIP status information remains accessible when a router
has non-{version} interfaces.

"""

import infamy
import infamy.route as route
from infamy.util import until, parallel


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.add_argument("--version", type=str.lower, choices=["ripv2", "ripng"])


PARAM = {
    "ripv2": {
        "af":        "ipv4",
        "len":       24,
        "hostlen":   32,
        "R1data":    "192.168.10.1",
        "R1link":    "192.168.50.1",
        "R1lo":      "192.168.100.1",
        "R2link":    "192.168.50.2",
        "R2data":    "192.168.60.1",
        "R2lo":      "192.168.200.1",
        "blackhole": "192.168.33.1",
        "R1net":     "192.168.10.0/24",
        "R2net":     "192.168.60.0/24",
        "PC1":       "192.168.10.2",
        "PC2":       "192.168.60.2",
    },
    "ripng": {
        "af":        "ipv6",
        "len":       64,
        "hostlen":   128,
        "R1data":    "2001:db8:10::1",
        "R1link":    "2001:db8:50::1",
        "R1lo":      "2001:db8:100::1",
        "R2link":    "2001:db8:50::2",
        "R2data":    "2001:db8:60::1",
        "R2lo":      "2001:db8:200::1",
        "blackhole": "2001:db8:33::1",
        "R1net":     "2001:db8:10::/64",
        "R2net":     "2001:db8:60::/64",
        "PC1":       "2001:db8:10::2",
        "PC2":       "2001:db8:60::2",
    },
}


def iface(p, name, addr, prefix_length=None, forwarding=True):
    """Interface with a single address of the tested address family"""
    ip = {"address": [{"ip": addr, "prefix-length": prefix_length or p["len"]}]}
    if forwarding:
        ip["forwarding"] = True

    return {"name": name, "enabled": True, p["af"]: ip}


def rip(p, link, redistribute):
    return {
        "type": f"infix-routing:{p['version']}",
        "name": "default",
        "rip": {
            "timers": {
                "update-interval": 5,
                "invalid-interval": 15,
                "flush-interval": 20
            },
            "redistribute": {
                "redistribute": [{"protocol": proto} for proto in redistribute]
            },
            "interfaces": {
                "interface": [{
                    "interface": link
                }]
            }
        }
    }


def config_target1(target, data, link, p):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    iface(p, data, p["R1data"]),
                    iface(p, link, p["R1link"]),
                    iface(p, "lo", p["R1lo"], p["hostlen"], forwarding=False)
                ]
            }
        },
        "ietf-system": {
            "system": {
                "hostname": "R1"
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": "infix-routing:static",
                        "name": "default",
                        "static-routes": {
                            p["af"]: {
                                "route": [{
                                    "destination-prefix": f"{p['blackhole']}/{p['hostlen']}",
                                    "next-hop": {
                                        "special-next-hop": "blackhole"
                                    }
                                }]
                            }
                        }
                    }, rip(p, link, ["static", "connected"])]
                }
            }
        }
    })


def config_target2(target, link, data, p):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    iface(p, link, p["R2link"]),
                    iface(p, data, p["R2data"]),
                    iface(p, "lo", p["R2lo"], p["hostlen"], forwarding=False)
                ]
            }
        },
        "ietf-system": {
            "system": {
                "hostname": "R2"
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [rip(p, link, ["connected"])]
                }
            }
        }
    })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env(args=ArgumentParser())
        version = env.args.version
        param = PARAM[version] | {"version": version}
        af, plen = param["af"], param["hostlen"]

        R1, R2 = parallel(lambda: env.attach("R1", "mgmt"),
                          lambda: env.attach("R2", "mgmt"))

    with test.step("Configure targets"):
        _, R1data = env.ltop.xlate("R1", "data")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R1link = env.ltop.xlate("R1", "link")
        _, R2data = env.ltop.xlate("R2", "data")

        parallel(lambda: config_target1(R1, R1data, R1link, param),
                 lambda: config_target2(R2, R2link, R2data, param))

    with test.step(f"Wait for {version} routes to be exchanged"):
        print("Waiting for RIP routes to propagate...")
        # R1 should learn R2's loopback
        until(lambda: route.route_exist(R1, f"{param['R2lo']}/{plen}", af=af, proto="ietf-rip:rip"), attempts=40)
        # R2 should learn R1's loopback
        until(lambda: route.route_exist(R2, f"{param['R1lo']}/{plen}", af=af, proto="ietf-rip:rip"), attempts=40)
        # R2 should learn R1's static route (redistributed)
        until(lambda: route.route_exist(R2, f"{param['blackhole']}/{plen}", af=af, proto="ietf-rip:rip"), attempts=40)
        until(lambda: route.route_exist(R2, param["R1net"], af=af, proto="ietf-rip:rip"), attempts=40)
        until(lambda: route.route_exist(R1, param["R2net"], af=af, proto="ietf-rip:rip"), attempts=40)

    with test.step(f"Test connectivity from PC:data1 to R2 loopback via {version}"):
        _, hport0 = env.ltop.xlate("PC", "data1")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip(param["PC1"], prefix_length=param["len"], proto=af)
            ns0.addroute(f"{param['R2lo']}/{plen}", param["R1data"], proto=af)
            ns0.must_reach(param["R2lo"])

    with test.step(f"Test connectivity from PC:data2 to R1 loopback via {version}"):
        _, hport1 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport1) as ns1:
            ns1.addip(param["PC2"], prefix_length=param["len"], proto=af)
            ns1.addroute(f"{param['R1lo']}/{plen}", param["R2data"], proto=af)
            ns1.must_reach(param["R1lo"])

    test.succeed()
