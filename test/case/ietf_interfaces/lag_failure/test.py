#!/usr/bin/env python3
r"""Link Aggregation Silent Failure

Verify communication over a link aggregate in static and LACP mode when
member links stop passing traffic without any carrier loss.  In static
mode the ARP monitor is used in both ends of the lag, in LACP mode this
is not necessary, and must in fact be disabled.

.Logical network setup, link breakers (lb1 & lb2) here managed by host PC
image::lag-failure.svg[]

The host verifies connectivity with dut2 via dut1 over the aggregate for
each test step using the `mon` interface.

"""
import infamy
from infamy.netns import TPMR
from infamy.util import parallel, until


class LinkBreaker:
    """Encapsulates basic, dumb link-breaking ops over SSH."""

    def __init__(self, env, ns):
        self.ns = ns
        self.lb1 = TPMR(env.ltop.xlate("host", "lb1a")[1],
                        env.ltop.xlate("host", "lb1b")[1]).start()
        self.lb2 = TPMR(env.ltop.xlate("host", "lb2a")[1],
                        env.ltop.xlate("host", "lb2b")[1]).start()

    def forward(self, lb1, lb2):
        """Set link breakers in forwarding or blocking state."""
        getattr(self.lb1, lb1)()
        getattr(self.lb2, lb2)()

    def fail_check(self, peer):
        """Verify connectivity with a given peer during failure."""
        sequence = [
            ("forward", "forward"),
            ("block",   "forward"),
            ("forward", "block"),
            ("forward", "forward")
        ]

        print(f"{'LB1':<8} | {'LB2':<8} | {'Status':<8}")
        print("---------|----------|---------")

        for lb1, lb2 in sequence:
            try:
                print(f"{lb1:<8} | {lb2:<8} | {'...':<8}", end="\r# ")
                self.forward(lb1, lb2)
                self.ns.must_reach(peer, timeout=10)
                print(f"{lb1:<8} | {lb2:<8} | {'OK':<8}")
            except Exception as e:
                print(f"{lb1:<8} | {lb2:<8} | {'FAIL':<8}")
                breakpoint()
                print(f"\nError encountered: {e}")
                print(f"Link breakers were in state: LB1='{lb1}', LB2='{lb2}'")
                raise

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
                "enabled": True,
                "ipv4": {
                    "address": [
                        {
                            "ip": "192.168.2.41",
                            "prefix-length": 24
                        }
                    ]
                }
            }, {
                "name": dmon,
                "bridge-port": {
                    "bridge": "br0",
                }
            }, {
                "name": "lag0",
                "bridge-port": {
                    "bridge": "br0",
                }
            }
        ]
        if mode == "static":
            extra += [
                {
                    "name": "lag0",
                    "lag": {
                        "arp-monitor": {
                            "interval": 100,
                            "peer": [
                                "192.168.2.42"
                            ]
                        }
                    }
                }
            ]
        else:
            extra += [
                {
                    "name": "lag0",
                    "lag": {
                        "lacp": {
                            "rate": "fast"
                        },
                        "arp-monitor": {
                            "interval": 0
                        },
                        "link-monitor": {
                            "interval": 100
                        }
                    }
                }
            ]
    else:
        # dut2
        extra = [
            {
                "name": "lag0",
                "ipv4": {
                    "address": [
                        {
                            "ip": "192.168.2.42",
                            "prefix-length": 24
                        }
                    ]
                }
            }
        ]
        if mode == "static":
            extra += [
                {
                    "name": "lag0",
                    "lag": {
                        "arp-monitor": {
                            "interval": 100,
                            "peer": [
                                "192.168.2.41"
                            ]
                        }
                    }
                }
            ]
        else:
            extra += [
                {
                    "name": "lag0",
                    "lag": {
                        "lacp": {
                            "rate": "fast"
                        },
                        "arp-monitor": {
                            "interval": 0
                        },
                        "link-monitor": {
                            "interval": 100
                        }
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
                        "lag-port": {
                            "lag": "lag0"
                        }
                    }, {
                        "name": link2,
                        "enabled": True,
                        "lag-port": {
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
        lb = LinkBreaker(env, ns)
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
