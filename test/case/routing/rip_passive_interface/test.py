#!/usr/bin/env python3
"""{version} Passive Interface

Verifies {version} passive interface functionality.  A passive interface means
that RIP will include the interface's network in routing updates but will not
send or receive RIP updates on that interface.

R1 has two {version}-enabled interfaces:
- data: Passive interface (network advertised but no updates sent/received)
- link: Active interface (RIP updates exchanged with R2)

R2 should learn about R1's data network from R1 via the link interface, even
though the data interface is passive.

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
        "af":     "ipv4",
        "len":    24,
        "R1data": "192.168.10.1",
        "R1link": "192.168.50.1",
        "R2link": "192.168.50.2",
        "R1net":  "192.168.10.0/24",
        "PC":     "192.168.10.2",
    },
    "ripng": {
        "af":     "ipv6",
        "len":    64,
        "R1data": "2001:db8:10::1",
        "R1link": "2001:db8:50::1",
        "R2link": "2001:db8:50::2",
        "R1net":  "2001:db8:10::/64",
        "PC":     "2001:db8:10::2",
    },
}


def iface(p, name, addr):
    """Interface with a single address of the tested address family"""
    return {
        "name": name,
        "enabled": True,
        p["af"]: {
            "forwarding": True,
            "address": [{"ip": addr, "prefix-length": p["len"]}]
        }
    }


def rip(p, interfaces):
    return {
        "type": f"infix-routing:{p['version']}",
        "name": "default",
        "rip": {
            "timers": {
                "update-interval": 5,
                "invalid-interval": 15,
                "flush-interval": 20
            },
            "interfaces": {
                "interface": interfaces
            }
        }
    }


def config_target1(target, data, link, p):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    iface(p, data, p["R1data"]),
                    iface(p, link, p["R1link"])
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
                    "control-plane-protocol": [rip(p, [{
                        "interface": data,
                        "passive": None
                    }, {
                        "interface": link
                    }])]
                }
            }
        }
    })


def config_target2(target, link, p):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [iface(p, link, p["R2link"])]
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
                    "control-plane-protocol": [rip(p, [{"interface": link}])]
                }
            }
        }
    })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env(args=ArgumentParser())
        version = env.args.version
        param = PARAM[version] | {"version": version}
        af = param["af"]

        R1, R2 = parallel(lambda: env.attach("R1", "mgmt"),
                          lambda: env.attach("R2", "mgmt"))

    with test.step("Configure targets"):
        _, R1data = env.ltop.xlate("R1", "data")
        _, R2link = env.ltop.xlate("R2", "link")
        _, R1link = env.ltop.xlate("R1", "link")

        parallel(lambda: config_target1(R1, R1data, R1link, param),
                 lambda: config_target2(R2, R2link, param))

    with test.step(f"Wait for {version} to exchange routes"):
        print("Waiting for RIP routes to propagate...")
        # R2 should learn about R1's passive interface network even though it
        # is passive, R1 should still advertise it
        until(lambda: route.route_exist(R2, param["R1net"], af=af, proto="ietf-rip:rip"), attempts=40)

    with test.step("Verify connectivity to passive interface network"):
        # Test that we can reach the passive interface from PC
        _, hport0 = env.ltop.xlate("PC", "data")
        with infamy.IsolatedMacVlan(hport0) as ns0:
            ns0.addip(param["PC"], prefix_length=param["len"], proto=af)
            # No need for route since we're on the same network
            ns0.must_reach(param["R1data"])

    test.succeed()
