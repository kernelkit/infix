#!/usr/bin/env python3
r"""Ling Aggregation Basic

Verify communication over a link aggregate in static and LACP operating
modes during basic failure scenarios.

.Internal network setup, PC verifies connectivity with dut2 via dut1
image::lag-basic.svg[Internal networks]

"""
import infamy
from infamy.util import parallel, until


class DumbLinkBreaker:
    """Encapsulates basic, dumb link-breaking ops over SSH."""

    def __init__(self, env, dut, ns):
        self.env = env
        self.dut = dut
        self.ns = ns
        self.tgt = env.attach(dut, "mgmt", "ssh")

    def set_link(self, link, updown):
        """Set link up or down."""
        _, port = self.env.ltop.xlate(self.dut, link)
        self.tgt.runsh(f"ip link set {port} {updown}")

    def fail_check(self, peer):
        """Verify connectivity with a given peer during failure."""
        sequence = [
            [("link1", "up"),   ("link2", "up")],
            [("link1", "down"), ("link2", "up")],
            [("link1", "up"),   ("link2", "down")],
            [("link1", "up"),   ("link2", "up")]
        ]

        for state in sequence:
            for link, updown in state:
                self.set_link(link, updown)
            self.ns.must_reach(peer)


def lag_init(dut, mode):
    """Set up link aggregate on dut"""
    _, link1 = env.ltop.xlate(dut.name, "link1")
    _, link2 = env.ltop.xlate(dut.name, "link2")

    try:
        _, dmon = env.ltop.xlate(dut.name, "mon")
    except TypeError:
        dmon = None

    if dmon:
        # dut1
        extra = [
            {
                "name": "br0",
                "type": "infix-if-type:bridge",
                "enabled": True
            }, {
                "name": dmon,
                "infix-interfaces:bridge-port": {
                    "bridge": "br0",
                }
            }, {
                "name": "lag0",
                "infix-interfaces:bridge-port": {
                    "bridge": "br0",
                }
            }
        ]
    else:
        # dut2
        extra = [
            {
                "name": "lag0",
                "ietf-ip:ipv4": {
                    "address": [
                        {
                            "ip": "192.168.2.42",
                            "prefix-length": 24
                        }
                    ]
                }
            }
        ]

    dut.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": "lag0",
                        "type": "infix-if-type:lag",
                        "enabled": True,
                        "lag": {
                            "mode": mode
                        }
                    }, {
                        "name": link1,
                        "enabled": True,
                        "infix-interfaces:lag-port": {
                            "lag": "lag0"
                        }
                    }, {
                        "name": link2,
                        "enabled": True,
                        "infix-interfaces:lag-port": {
                            "lag": "lag0"
                        }
                    }
                ] + extra
            }
        }
    })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        dut1 = env.attach("dut1", "mgmt")
        dut2 = env.attach("dut2", "mgmt")

    _, mon = env.ltop.xlate("host", "mon")
    with infamy.IsolatedMacVlan(mon) as ns:
        lb = DumbLinkBreaker(env, "dut1", ns)
        ns.addip("192.168.2.1")

        with test.step("Set up static link aggregate, lag0, on dut1 and dut2"):
            parallel(lambda: lag_init(dut1, "static"),
                     lambda: lag_init(dut2, "static"))

        with test.step("Verify failure modes for static mode"):
            lb.fail_check("192.168.2.42")

        with test.step("Set up LACP link aggregate, lag0, on dut1 and dut2"):
            parallel(lambda: lag_init(dut1, "lacp"),
                     lambda: lag_init(dut2, "lacp"))

        with test.step("Verify failure modes for lacp mode"):
            lb.fail_check("192.168.2.42")

    test.succeed()
