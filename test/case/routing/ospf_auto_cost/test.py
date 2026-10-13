#!/usr/bin/env python3
"""{version} Auto-Cost Reference Bandwidth

Verifies that the {version} interface cost follows the configured auto-cost
reference bandwidth.  The reference bandwidth must be the same on all
routers in an OSPF domain, so it is changed on both R1 and R2.

The cost of an interface is the reference bandwidth divided by the link
speed, with 1 as the lowest cost.  A link of unknown speed, e.g., a virtual
link, counts as 10000 Mbit/s.  On a 1 Gbit/s link the cost is:

|===
| Reference bandwidth      | Cost
| 100000 Mbit/s (default)  | 100
| 10000 Mbit/s             | 10
| 1000 Mbit/s              | 1
|===

The test reads the link speed on each router to calculate the expected
cost, and reads the reference bandwidth back from the operational
datastore.
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
        "af":     "ipv4",
        "len":    24,
        "R1link": "192.168.50.1",
        "R2link": "192.168.50.2",
    },
    "ospfv3": {
        "af":     "ipv6",
        "len":    64,
        "R1link": "2001:db8:50::1",
        "R2link": "2001:db8:50::2",
    },
}

ROUTER_ID = {"R1": "1.1.1.1", "R2": "2.2.2.2"}

# Frr uses this link speed (Mbit/s) when the speed is unknown
UNKNOWN_SPEED = 10000


def config_target(target, name, link, p, proto):
    target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": link,
                    "enabled": True,
                    p["af"]: {
                        "forwarding": True,
                        "address": [{
                            "ip": p[f"{name}link"],
                            "prefix-length": p["len"]
                        }]
                    }
                }]
            }
        },
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
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
                    }]
                }
            }
        }
    })


def set_reference_bandwidth(target, refbw, proto):
    target.put_config_dicts({
        "ietf-routing": {
            "routing": {
                "control-plane-protocols": {
                    "control-plane-protocol": [{
                        "type": proto,
                        "name": "default",
                        "ospf": {
                            "auto-cost": {
                                "reference-bandwidth": refbw
                            }
                        }
                    }]
                }
            }
        }
    })


def link_speed(target, link):
    """Link speed in Mbit/s, as used by Frr to calculate the cost"""
    speed = target.get_iface(link).get("speed")
    if not speed:
        return UNKNOWN_SPEED

    return int(speed) // 1000000


def expected_cost(refbw, speed):
    """Same rounding as Frr, with 1 as the lowest cost"""
    return max(1, int(refbw / speed + 0.5))


def wait_adjacency(target, link, neighbor, proto):
    until(lambda: route.ospf_get_neighbor(target, "0.0.0.0", link, ROUTER_ID[neighbor],
                                          proto=proto), attempts=200)


def verify(target, link, refbw, proto):
    speed = link_speed(target, link)
    cost = expected_cost(refbw, speed)
    print(f"Expecting reference bandwidth {refbw} Mbit/s and cost {cost} "
          f"on {link} ({speed} Mbit/s)")

    until(lambda: route.ospf_get_reference_bandwidth(target, proto) == refbw and
          route.ospf_get_interface_cost(target, "0.0.0.0", link, proto) == cost)


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env(args=ArgumentParser())
        version = env.args.version
        param = PARAM[version]
        proto = f"infix-routing:{version}"

        R1, R2 = parallel(lambda: env.attach("R1", "mgmt"),
                          lambda: env.attach("R2", "mgmt"))
        route.skip_unless_supported(test, "ospf", R1, R2)

        _, R1link = env.ltop.xlate("R1", "link")
        _, R2link = env.ltop.xlate("R2", "link")

    def set_both(refbw):
        parallel(lambda: set_reference_bandwidth(R1, refbw, proto),
                 lambda: set_reference_bandwidth(R2, refbw, proto))

    def verify_both(refbw):
        parallel(lambda: verify(R1, R1link, refbw, proto),
                 lambda: verify(R2, R2link, refbw, proto))

    with test.step(f"Configure {version} on R1 and R2"):
        parallel(lambda: config_target(R1, "R1", R1link, param, proto),
                 lambda: config_target(R2, "R2", R2link, param, proto))

    with test.step(f"Wait for {version} adjacency between R1 and R2"):
        parallel(lambda: wait_adjacency(R1, R1link, "R2", proto),
                 lambda: wait_adjacency(R2, R2link, "R1", proto))

    with test.step("Verify default reference bandwidth 100000 Mbit/s and link cost"):
        verify_both(100000)

    with test.step("Set reference bandwidth 10000 Mbit/s on R1 and R2"):
        set_both(10000)

    with test.step("Verify reference bandwidth 10000 Mbit/s and link cost"):
        verify_both(10000)

    with test.step("Set reference bandwidth 1000 Mbit/s on R1 and R2"):
        set_both(1000)

    with test.step("Verify reference bandwidth 1000 Mbit/s and link cost"):
        verify_both(1000)

    test.succeed()
