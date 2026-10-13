#!/usr/bin/env python3
"""{proto} Dual Stack

The status of each {proto} instance must stay visible in the operational
datastore when the IPv4 and IPv6 versions run at the same time.

R1 and R2 have an IPv4 and an IPv6 address on the link between them, with
{proto} for both address families on that link.  The test waits until the
operational datastore shows the neighbor in each instance, a full
adjacency for OSPF and a known peer for RIP.  A RIP router does not send
the link prefix back on the link, so each router also redistributes a
loopback address to have something to advertise.
"""

import infamy
import infamy.route as route
from infamy.util import until, parallel


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.add_argument("--proto", type=str.lower, choices=["ospf", "rip"])


ROUTER_ID = {"R1": "1.1.1.1", "R2": "2.2.2.2"}
IPV4 = {"R1": "192.168.50.1", "R2": "192.168.50.2"}
IPV6 = {"R1": "2001:db8:50::1", "R2": "2001:db8:50::2"}
LO4 = {"R1": "192.168.100.1", "R2": "192.168.200.1"}
LO6 = {"R1": "2001:db8:100::1", "R2": "2001:db8:200::1"}

# IPv4 and IPv6 instance type for each protocol
TYPES = {
    "ospf": ["infix-routing:ospfv2", "infix-routing:ospfv3"],
    "rip":  ["infix-routing:ripv2", "infix-routing:ripng"],
}


def ospf_instance(proto, name, link):
    return {
        "type": proto,
        "name": "default",
        "ospf": {
            "explicit-router-id": ROUTER_ID[name],
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
    }


def rip_instance(proto, name, link):
    return {
        "type": proto,
        "name": "default",
        "rip": {
            "timers": {
                "update-interval": 5,
                "invalid-interval": 15,
                "flush-interval": 20
            },
            "redistribute": {
                "redistribute": [{"protocol": "connected"}]
            },
            "interfaces": {
                "interface": [{
                    "interface": link
                }]
            }
        }
    }


def config_target(target, name, link, proto):
    instance = ospf_instance if proto == "ospf" else rip_instance
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": link,
                    "enabled": True,
                    "ipv4": {
                        "forwarding": True,
                        "address": [{
                            "ip": IPV4[name],
                            "prefix-length": 24
                        }]
                    },
                    "ipv6": {
                        "forwarding": True,
                        "address": [{
                            "ip": IPV6[name],
                            "prefix-length": 64
                        }]
                    }
                }, {
                    "name": "lo",
                    "ipv4": {
                        "address": [{
                            "ip": LO4[name],
                            "prefix-length": 32
                        }]
                    },
                    "ipv6": {
                        "address": [{
                            "ip": LO6[name],
                            "prefix-length": 128
                        }]
                    }
                }]
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [instance(t, name, link) for t in TYPES[proto]]
                }
            }
        }
    })


def has_neighbor(target, link, neighbor, proto, version):
    if proto == "ospf":
        return route.ospf_get_neighbor(target, "0.0.0.0", link, ROUTER_ID[neighbor],
                                       proto=version)

    # RIPng peers are known by their link-local address only
    return route.rip_has_neighbors(target, proto=version)


def wait_neighbor(target, link, neighbor, proto, version):
    until(lambda: has_neighbor(target, link, neighbor, proto, version), attempts=200)


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env(args=ArgumentParser())
        proto = env.args.proto
        ipv4, ipv6 = TYPES[proto]

        R1, R2 = parallel(lambda: env.attach("R1", "mgmt"),
                          lambda: env.attach("R2", "mgmt"))
        route.skip_unless_supported(test, proto, R1, R2)

        _, R1link = env.ltop.xlate("R1", "link")
        _, R2link = env.ltop.xlate("R2", "link")

    with test.step(f"Configure {proto} for IPv4 and IPv6 on R1 and R2"):
        parallel(lambda: config_target(R1, "R1", R1link, proto),
                 lambda: config_target(R2, "R2", R2link, proto))

    with test.step(f"Wait for the IPv4 {proto} neighbor on R1 and R2"):
        parallel(lambda: wait_neighbor(R1, R1link, "R2", proto, ipv4),
                 lambda: wait_neighbor(R2, R2link, "R1", proto, ipv4))

    with test.step(f"Wait for the IPv6 {proto} neighbor on R1 and R2"):
        parallel(lambda: wait_neighbor(R1, R1link, "R2", proto, ipv6),
                 lambda: wait_neighbor(R2, R2link, "R1", proto, ipv6))

    test.succeed()
