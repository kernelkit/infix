#!/usr/bin/env python3
"""
{version} Default route advertise

Verify _default-route-advertising_ in {version}, sometimes called 'redistribute
origin'.  Verify both 'always' (regardless of whether a local default route
exists) and the conditional mode (only redistribute when a local default route
exists).

R1 has a default route via its data interface and enables
default-route-advertise, so R2 learns a default route via {version}.  When
R1:data is taken down the local default is withdrawn and R2 loses the default
route, unless 'always' is set.
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
The OSPFv3 run uses the same topology with 2001:db8:10::/64,
2001:db8:20::/64 and 2001:db8:50::/64 respectively.
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.add_argument("--version", type=str.lower, choices=["ospfv2", "ospfv3"])


PARAM = {
    "ospfv2": {
        "af":      "ipv4",
        "len":     24,
        "hostlen": 32,
        "default": "0.0.0.0/0",
        "dummy":   "10.10.10.10",
        "R1data":  "192.168.10.1",
        "R1gw":    "192.168.10.2",
        "R1link":  "192.168.50.1",
        "R1lo":    "192.168.100.1",
        "R2link":  "192.168.50.2",
        "R2data":  "192.168.20.1",
        "R2net":   "192.168.20.0/24",
        "R2lo":    "192.168.200.1",
        "PC":      "192.168.20.2",
    },
    "ospfv3": {
        "af":      "ipv6",
        "len":     64,
        "hostlen": 128,
        "default": "::/0",
        "dummy":   "2001:db8:cafe::10",
        "R1data":  "2001:db8:10::1",
        "R1gw":    "2001:db8:10::2",
        "R1link":  "2001:db8:50::1",
        "R1lo":    "2001:db8:100::1",
        "R2link":  "2001:db8:50::2",
        "R2data":  "2001:db8:20::1",
        "R2net":   "2001:db8:20::/64",
        "R2lo":    "2001:db8:200::1",
        "PC":      "2001:db8:20::2",
    },
}


def iface(p, name, addr, prefix_length=None, forwarding=True, **kwargs):
    """Interface with a single address of the tested address family"""
    ip = {"address": [{"ip": addr, "prefix-length": prefix_length or p["len"]}]}
    if forwarding:
        ip["forwarding"] = True

    return {"name": name, "enabled": True, p["af"]: ip} | kwargs


def config_target1(target, data, link, p):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    iface(p, "dummy0", p["dummy"], p["hostlen"], forwarding=False,
                          type="infix-if-type:dummy"),
                    iface(p, data, p["R1data"]),
                    iface(p, link, p["R1link"]),
                    iface(p, "lo", p["R1lo"], p["hostlen"], forwarding=False)
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
                                p["af"]: {
                                    "route": [{
                                        "destination-prefix": p["default"],
                                        "next-hop": {
                                            "next-hop-address": p["R1gw"]
                                        }
                                    }]
                                }
                            }
                        },
                        {
                            "type": f"infix-routing:{p['version']}",
                            "name": "default",
                            "ospf": {
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


def config_target2(target, data, link, p):
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


def set_redistribute_default_always(target, p):
    target.put_config_dicts({
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": f"infix-routing:{p['version']}",
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
        env = infamy.Env(args=ArgumentParser())
        version = env.args.version
        param = PARAM[version] | {"version": version}
        af, proto = param["af"], f"ietf-ospf:{version}"
        instance = f"infix-routing:{version}"
        R1lo = f"{param['R1lo']}/{param['hostlen']}"
        R2lo = f"{param['R2lo']}/{param['hostlen']}"

        R1, R2 = parallel(lambda: env.attach("R1", "mgmt"),
                          lambda: env.attach("R2", "mgmt"))

    with test.step("Configure targets"):
        _, R1data = env.ltop.xlate("R1", "data")
        _, R2data = env.ltop.xlate("R2", "data")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R1link = env.ltop.xlate("R1", "link")

        parallel(lambda: config_target1(R1, R1data, R1link, param),
                 lambda: config_target2(R2, R2data, R2link, param))

    with test.step("Verify R1 has an active static default route"):
        until(lambda: route.route_exist(R1, param["default"], af=af, proto="ietf-routing:static",
                                        active_check=True), attempts=60)

    with test.step("Wait for all neighbors to peer"):
        until(lambda: route.ospf_get_neighbor(R1, "0.0.0.0", R1link, "2.2.2.2",
                                              proto=instance), attempts=200)
        until(lambda: route.ospf_get_neighbor(R2, "0.0.0.0", R2link, "1.1.1.1",
                                              proto=instance), attempts=200)

    with test.step(f"Verify R2 has a default route and R1's loopback from {version}"):
        print("Waiting for OSPF routes...")
        until(lambda: route.route_exist(R2, param["default"], af=af, proto=proto, active_check=True), attempts=200)
        until(lambda: route.route_exist(R2, R1lo, af=af, proto=proto, active_check=True), attempts=200)
        until(lambda: route.route_exist(R1, R2lo, af=af, proto=proto, active_check=True), attempts=200)
        until(lambda: route.route_exist(R1, param["R2net"], af=af, proto=proto, active_check=True), attempts=200)

    with test.step("Verify connectivity from PC:data2 to R1's dummy interface"):
        _, hport0 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip(param["PC"], prefix_length=param["len"], proto=af)
            ns0.addroute(param["default"], param["R2data"], proto=af)
            ns0.must_reach(param["dummy"], timeout=15)

    with test.step("Disable link PC:data1 <--> R1:data (take default gateway down)"):
        disable_interface(R1, R1data)

    with test.step(f"Verify R2 loses the default route but keeps R1's loopback from {version}"):
        until(lambda: route.route_exist(R2, R1lo, af=af, proto=proto, active_check=True), attempts=200)
        until(lambda: route.route_exist(R1, R2lo, af=af, proto=proto, active_check=True), attempts=200)
        until(lambda: route.route_exist(R2, param["default"], af=af, proto=proto) == False, attempts=200)

    with test.step("Verify no connectivity from PC:data2 to R1's dummy interface"):
        _, hport0 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip(param["PC"], prefix_length=param["len"], proto=af)
            ns0.addroute(param["default"], param["R2data"], proto=af)
            ns0.must_not_reach(param["dummy"])

    with test.step("Enable redistribute default route 'always' on R1"):
        set_redistribute_default_always(R1, param)

    with test.step("Wait for all neighbors to peer"):
        until(lambda: route.ospf_get_neighbor(R1, "0.0.0.0", R1link, "2.2.2.2",
                                              proto=instance), attempts=200)
        until(lambda: route.ospf_get_neighbor(R2, "0.0.0.0", R2link, "1.1.1.1",
                                              proto=instance), attempts=200)

    with test.step(f"Verify R2 has a default route and R1's loopback from {version}"):
        print("Waiting for OSPF routes...")
        until(lambda: route.route_exist(R2, R1lo, af=af, proto=proto, active_check=True), attempts=200)
        until(lambda: route.route_exist(R1, R2lo, af=af, proto=proto, active_check=True), attempts=200)
        until(lambda: route.route_exist(R1, param["R2net"], af=af, proto=proto, active_check=True), attempts=200)
        until(lambda: route.route_exist(R2, param["default"], af=af, proto=proto, active_check=True), attempts=200)

    with test.step("Verify connectivity from PC:data2 to R1's dummy interface"):
        _, hport0 = env.ltop.xlate("PC", "data2")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip(param["PC"], prefix_length=param["len"], proto=af)
            ns0.addroute(param["default"], param["R2data"], proto=af)
            ns0.must_reach(param["dummy"], timeout=15)

    test.succeed()
