#!/usr/bin/env python3
"""{version} Redistribution

Verifies that {version} can redistribute routes from other protocols.

Topology:
- R1: Gateway router running both RIP and OSPF
  - RIP interface to R2
  - OSPF interface to R3
  - Redistributes OSPF routes into RIP
  - Redistributes RIP routes into OSPF

- R2: RIP-only router with a loopback

- R3: OSPF-only router with a loopback

Expected behavior:
- R2 (RIP) should learn R3's OSPF loopback via RIP redistribution
- R3 (OSPF) should learn R2's RIP loopback via OSPF redistribution

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
        "af":      "ipv4",
        "len":     24,
        "hostlen": 32,
        "ospf":    "ospfv2",
        "R1rip":   "192.168.50.1",
        "R1ospf":  "192.168.60.1",
        "R2link":  "192.168.50.2",
        "R2lo":    "192.168.200.1",
        "R3link":  "192.168.60.2",
        "R3lo":    "192.168.100.1",
        "ospfnet": "192.168.60.0/24",
    },
    "ripng": {
        "af":      "ipv6",
        "len":     64,
        "hostlen": 128,
        "ospf":    "ospfv3",
        "router-id": {"R1": "1.1.1.1", "R3": "3.3.3.3"},
        "R1rip":   "2001:db8:50::1",
        "R1ospf":  "2001:db8:60::1",
        "R2link":  "2001:db8:50::2",
        "R2lo":    "2001:db8:200::1",
        "R3link":  "2001:db8:60::2",
        "R3lo":    "2001:db8:100::1",
        "ospfnet": "2001:db8:60::/64",
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


def ospf(p, name, link, redistribute):
    conf = {}
    if "router-id" in p:
        conf["explicit-router-id"] = p["router-id"][name]

    conf |= {
        "redistribute": {
            "redistribute": [{"protocol": proto} for proto in redistribute]
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

    return {
        "type": f"infix-routing:{p['ospf']}",
        "name": "default",
        "ospf": conf
    }


def config_r1_gateway(target, rip_link, ospf_link, p):
    """Configure R1 as gateway running both RIP and OSPF"""
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    iface(p, rip_link, p["R1rip"]),
                    iface(p, ospf_link, p["R1ospf"])
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
                    "control-plane-protocol": [
                        rip(p, rip_link, ["ospf", "connected"]),
                        ospf(p, "R1", ospf_link, ["rip", "connected"])
                    ]
                }
            }
        }
    })


def config_r2_rip(target, link, p):
    """Configure R2 with RIP only"""
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    iface(p, link, p["R2link"]),
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


def config_r3_ospf(target, link, p):
    """Configure R3 with OSPF only"""
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    iface(p, link, p["R3link"]),
                    iface(p, "lo", p["R3lo"], p["hostlen"], forwarding=False)
                ]
            }
        },
        "ietf-system": {
            "system": {
                "hostname": "R3"
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [ospf(p, "R3", link, ["connected"])]
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
        ospf_proto = f"ietf-ospf:{param['ospf']}"
        R2lo = f"{param['R2lo']}/{plen}"
        R3lo = f"{param['R3lo']}/{plen}"

        R1, R2, R3 = parallel(lambda: env.attach("R1", "mgmt"),
                              lambda: env.attach("R2", "mgmt"),
                              lambda: env.attach("R3", "mgmt"))

    with test.step("Configure routers"):
        _, R1rip = env.ltop.xlate("R1", "rip")
        _, R1ospf = env.ltop.xlate("R1", "ospf")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R3link = env.ltop.xlate("R3", "link")

        parallel(lambda: config_r1_gateway(R1, R1rip, R1ospf, param),
                 lambda: config_r2_rip(R2, R2link, param),
                 lambda: config_r3_ospf(R3, R3link, param))

    with test.step("Wait for OSPF to converge on R1-R3 link"):
        print("Waiting for OSPF convergence...")
        # R1 should learn R3's loopback via OSPF
        until(lambda: route.route_exist(R1, R3lo, af=af, proto=ospf_proto), attempts=40)
        # R3 should learn R1's OSPF link via OSPF
        until(lambda: route.route_exist(R3, param["ospfnet"], af=af, proto=ospf_proto), attempts=40)

    with test.step(f"Wait for {version} to converge on R1-R2 link"):
        print("Waiting for RIP convergence...")
        # R1 should learn R2's loopback via RIP
        until(lambda: route.route_exist(R1, R2lo, af=af, proto="ietf-rip:rip"), attempts=40)
        # R2 should learn R1's OSPF link via RIP (redistributed from connected)
        until(lambda: route.route_exist(R2, param["ospfnet"], af=af, proto="ietf-rip:rip"), attempts=40)

    with test.step(f"Verify R2 ({version}) learns R3's OSPF routes via redistribution"):
        print("Checking OSPF->RIP redistribution...")
        # R2 should learn R3's loopback (OSPF route) via RIP redistribution on R1
        until(lambda: route.route_exist(R2, R3lo, af=af, proto="ietf-rip:rip"), attempts=40)

    with test.step(f"Verify R3 (OSPF) learns R2's {version} routes via redistribution"):
        print("Checking RIP->OSPF redistribution...")
        # R3 should learn R2's loopback (RIP route) via OSPF redistribution on R1
        until(lambda: route.route_exist(R3, R2lo, af=af, proto=ospf_proto), attempts=40)

    test.succeed()
