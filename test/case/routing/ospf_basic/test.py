#!/usr/bin/env python3
"""{version} Basic

Verifies basic {version} functionality by configuring two routers (R1 and R2)
with {version} on their interconnecting link.  The test ensures {version}
neighbors are established, routes are exchanged between the routers, and
end-to-end connectivity is achieved.

An end-device (HOST) is connected to R2 on an interface without {version}
enabled.  This verifies that {version} status information remains accessible
when a router has non-{version} interfaces.

Note: OSPFv3 has no IPv4 address to derive a router-id from, so an
explicit-router-id is configured when running OSPFv3.
"""

# TODO: Remove HOST node once Infamy supports unconnected ports in topologies

import infamy
import infamy.route as route
from infamy.util import until, parallel


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.add_argument("--version", type=str.lower, choices=["ospfv2", "ospfv3"])


PARAM = {
    "ospfv2": {
        "af":        "ipv4",
        "len":       24,
        "hostlen":   32,
        "R1data":    "192.168.10.1",
        "R1link":    "192.168.50.1",
        "R1lo":      "192.168.100.1",
        "R2link":    "192.168.50.2",
        "R2data":    "192.168.60.1",
        "R2lo":      "192.168.200.1",
        "HOSTlink":  "192.168.60.2",
        "blackhole": "192.168.33.1",
        "PC":        "192.168.10.2",
    },
    "ospfv3": {
        "af":        "ipv6",
        "len":       64,
        "hostlen":   128,
        "router-id": {"R1": "1.1.1.1", "R2": "2.2.2.2"},
        "R1data":    "2001:db8:10::1",
        "R1link":    "2001:db8:50::1",
        "R1lo":      "2001:db8:100::1",
        "R2link":    "2001:db8:50::2",
        "R2data":    "2001:db8:60::1",
        "R2lo":      "2001:db8:200::1",
        "HOSTlink":  "2001:db8:60::2",
        "blackhole": "2001:db8:33::1",
        "PC":        "2001:db8:10::2",
    },
}


def iface(p, name, addr, prefix_length=None, forwarding=True):
    """Interface with a single address of the tested address family"""
    ip = {"address": [{"ip": addr, "prefix-length": prefix_length or p["len"]}]}
    if forwarding:
        ip["forwarding"] = True

    return {"name": name, "enabled": True, p["af"]: ip}


def ospf(p, name):
    """OSPF instance settings, OSPFv3 needs an explicit router-id"""
    conf = {}
    if "router-id" in p:
        conf["explicit-router-id"] = p["router-id"][name]

    return conf


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
                    }, {
                        "type": f"infix-routing:{p['version']}",
                        "name": "default",
                        "ospf": {
                            **ospf(p, "R1"),
                            "redistribute": {
                                "redistribute": [{
                                    "protocol": "static"
                                }, {
                                    "protocol": "connected"
                                }]
                            },
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.0",
                                    "interfaces": {
                                        "interface": [{
                                            "enabled": True,
                                            "name": link,
                                            "hello-interval": 1,
                                            "dead-interval": 3
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
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": f"infix-routing:{p['version']}",
                        "name": "default",
                        "ospf": {
                            **ospf(p, "R2"),
                            "redistribute": {
                                "redistribute": [{
                                    "protocol": "connected"
                                }]
                            },
                            "areas": {
                                "area": [{
                                    "area-id": "0.0.0.0",
                                    "interfaces": {
                                        "interface": [{
                                            "enabled": True,
                                            "name": link,
                                            "hello-interval": 1,
                                            "dead-interval": 3
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


def config_host(target, link, p):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [iface(p, link, p["HOSTlink"], forwarding=False)]
            }
        }
    })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env(args=ArgumentParser())
        version = env.args.version
        param = PARAM[version] | {"version": version}

        R1, R2, HOST = parallel(lambda: env.attach("R1", "mgmt"),
                                lambda: env.attach("R2", "mgmt"),
                                lambda: env.attach("HOST", "mgmt"))

    with test.step("Configure targets"):
        _, R1data = env.ltop.xlate("R1", "data")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R1link = env.ltop.xlate("R1", "link")
        _, R2data = env.ltop.xlate("R2", "data")
        _, HOSTlink = env.ltop.xlate("HOST", "link")

        parallel(lambda: config_target1(R1, R1data, R1link, param),
                 lambda: config_target2(R2, R2link, R2data, param),
                 lambda: config_host(HOST, HOSTlink, param))

    with test.step(f"Wait for {version} routes"):
        af, plen = param["af"], param["hostlen"]
        proto = f"ietf-ospf:{version}"

        until(lambda: route.route_exist(R1, f"{param['R2lo']}/{plen}", af=af, proto=proto), attempts=200)
        until(lambda: route.route_exist(R2, f"{param['R1lo']}/{plen}", af=af, proto=proto), attempts=200)
        until(lambda: route.route_exist(R2, f"{param['blackhole']}/{plen}", af=af, proto=proto), attempts=200)

    with test.step(f"Verify R2 {version} neighbors with non-{version} interface"):
        # Regression test for #1169
        assert route.ospf_has_neighbors(R2, proto=f"infix-routing:{version}")

    with test.step("Test connectivity from PC:data to R2 loopback"):
        _, hport0 = env.ltop.xlate("PC", "data")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip(param["PC"], prefix_length=param["len"], proto=af)
            ns0.addroute(f"{param['R2lo']}/{plen}", param["R1data"], proto=af)
            ns0.must_reach(param["R2lo"])

    test.succeed()
