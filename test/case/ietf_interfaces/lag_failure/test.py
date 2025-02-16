#!/usr/bin/env python3
r"""LACP Aggregate w/ Degraded Link

Verify communication over an LACP link aggregate when individual member
links stop forwarding traffic, without carrier loss.

.Logical network setup, link breakers (lb1 & lb2) here managed by host PC
image::lag-failure.svg[]

The host verifies connectivity with dut2 via dut1 over the aggregate for
each failure mode step using the `mon` interface.

"""
from time import time
import infamy
import infamy.lag
from infamy.netns import TPMR
from infamy.util import parallel

IPH = "192.168.2.1"
IP1 = "192.168.2.41"
IP2 = "192.168.2.42"


class LinkBreaker:
    """Encapsulates TPMR based link-breakers."""

    def __init__(self, sys, netns):
        self.net = netns
        self.lb1 = TPMR(sys.ltop.xlate("host", "lb1a")[1],
                        sys.ltop.xlate("host", "lb1b")[1]).start()
        self.lb2 = TPMR(sys.ltop.xlate("host", "lb2a")[1],
                        sys.ltop.xlate("host", "lb2b")[1]).start()

    def forward(self, lb1, lb2):
        """Set link breakers in forwarding or blocking state."""
        getattr(self.lb1, lb1)()
        getattr(self.lb2, lb2)()

    def fail_check(self, peer):
        """Verify connectivity with a given peer during failure."""
        sequence = [
            ("forward", "forward"),
            ("forward", "block"),
            ("block",   "forward"),
            ("forward", "forward")
        ]

        total_start = time()
        print(f"{'LB1':<8} | {'LB2':<8} | {'Status':<8}")
        print("---------|----------|---------")

        for lb1, lb2 in sequence:
            state_start = time()
            try:
                print(f"{lb1:<8} | {lb2:<8} | {'...':<8}", end="\r# ")
                self.forward(lb1, lb2)
                self.net.must_reach(peer, timeout=30)
                print(f"{lb1:<8} | {lb2:<8} | {'OK':<8} in "
                      f"{time() - state_start:.2f}s")
            except Exception as e:
                print(f"{lb1:<8} | {lb2:<8} | {'FAIL':<8} after "
                      f"{time() - state_start:.2f}s")
                print(f"\nError encountered: {e}")
                print(f"Link breakers were in state: LB1='{lb1}', LB2='{lb2}'")
                raise

        print(f"Total time: {time() - total_start:.2f}s")


def net_init(host, addr):
    """Set up DUT network, dut1 bridges host port with lag0"""
    if host:
        net = [{
            "name": "br0",
            "type": "infix-if-type:bridge",
            "ipv4": {
                "address": [{"ip": addr, "prefix-length": 24}]
            }
        }, {
            "name": host,
            "bridge-port": {"bridge": "br0"}
        }, {
            "name": "lag0",
            "bridge-port": {"bridge": "br0"}
        }]
    else:
        net = [{
            "name": "lag0",
            "ipv4": {
                "address": [{"ip": addr, "prefix-length": 24}]
            }
        }]
    return net


def dut_init(dut, addr, peer):
    """Configure each DUT specific according to LAG mode and peer"""
    net = net_init(dut["mon"], addr)

    dut.put_config_dict("ietf-interfaces", {
        "interfaces": {
            "interface": [{
                "name": "lag0",
                "type": "infix-if-type:lag",
                "lag": {
                    "mode": "lacp",
                    "lacp": {"rate": "fast"},
                    "link-monitor": {"interval": 100}
                }
            }, {
                "name": dut["link1"],
                "lag-port": {"lag": "lag0"}
            }, {
                "name": dut["link2"],
                "lag-port": {"lag": "lag0"}
            }] + net
        }
    })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env(edge_mappings=infamy.lag.edge_mappings)
        dut1 = env.attach("dut1")
        dut2 = env.attach("dut2")

    _, mon = env.ltop.xlate("host", "mon")
    with infamy.IsolatedMacVlan(mon) as ns:
        lb = LinkBreaker(env, ns)
        ns.addip(IPH)

        print(f"Setting up lag0 in LACP mode between {dut1} and {dut2}")
        with test.step("Set up link aggregate, lag0, between dut1 and dut2"):
            parallel(lambda: dut_init(dut1, IP1, IP2),
                     lambda: dut_init(dut2, IP2, IP1))

        with test.step("Initial connectivity check ..."):
            ns.must_reach(IP2, timeout=30)

        with test.step("Verify failure modes"):
            lb.fail_check(IP2)

    test.succeed()
