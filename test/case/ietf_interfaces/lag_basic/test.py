#!/usr/bin/env python3
r"""Ling Aggregation Basic

Verify communication over a link aggregate in static and LACP operating
modes during basic failure scenarios.

.Internal network setup, PC verifies connectivity with dut2 via dut1
image::lag-basic.svg[Internal networks]

The host verifies connectivity with dut2 via dut1 over the aggregate for
each test step using the `mon` interface.

"""
from time import sleep, time
from datetime import datetime
import infamy
import infamy.lag
from infamy.util import parallel, until

class DumbLinkBreaker:
    """Encapsulates basic, dumb link-breaking ops over SSH."""

    def __init__(self, sys, dut, netns):
        self.env = sys
        self.dut = dut
        self.net = netns
        self.tgt = {}
        for i, (name, _) in dut.items():
            self.tgt[i] = env.attach(name, "mgmt", "ssh")

    def set_link(self, link, updown):
        """Set link up or down, verify before returning."""
        def set_and_verify(i):
            name, dut = self.dut[i]

            cmd = self.tgt[i].runsh(f"sudo ip link set {dut[link]} {updown}")
            if cmd.returncode:
                for out in [cmd.stdout, cmd.stderr]:
                    if out:
                        print(f"{name}: {out.rstrip()}")
                raise RuntimeError(f"{name}: failed setting {link} {updown}")

            for _ in range(10):
                sleep(0.1)
                check = self.tgt[i].runsh(f"ip link show {dut[link]}")
                if f"state {updown.upper()}" in check.stdout:
                    break
            else:
                raise RuntimeError(f"{name}: {dut[link]} did not go {updown}")

        parallel(*[lambda i=i: set_and_verify(i) for i in self.dut])

    def fail_check(self, peer):
        """Verify connectivity with peer during link failure."""
        sequence = [
            [("link1", "up"),   ("link2", "up")],
            [("link1", "down"), ("link2", "up")],
            [("link1", "up"),   ("link2", "down")],
            [("link1", "up"),   ("link2", "up")]
        ]

        total_start = time()
        for state in sequence:
            state_start = time()
            print(f"{datetime.now().strftime('%H:%M:%S.%f')[:-3]} {state}")

            for link, updown in state:
                self.set_link(link, updown)
            self.net.must_reach(peer, timeout=10)

            print(f"Completed in {time() - state_start:.2f}s")

        print(f"Total time: {time() - total_start:.2f}s")


def lag_init(mode):
    """Set up mode specific attributes for the LAG"""
    if mode == "lacp":
        lag = [{
            "name": "lag0",
            "lag": {"lacp": {"rate": "fast"}}
        }]
    else:
        lag = []
    return lag


def net_init(host, addr):
    """Set up DUT network, dut1 bridges host port with lag0"""
    if host:
        net = [{
            "name": "br0",
            "type": "infix-if-type:bridge",
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


def dut_init(dut, mode, addr):
    """Set up link aggregate on dut"""
    net = net_init(dut["mon"], addr)
    lag = lag_init(mode)

    dut.put_config_dict("ietf-interfaces", {
        "interfaces": {
            "interface": [{
                "name": "lag0",
                "type": "infix-if-type:lag",
                "lag": {
                    "mode": mode,
                    "link-monitor": {"interval": 100}
                }
            }, {
                "name": dut["link1"],
                "lag-port": {"lag": "lag0"}
            }, {
                "name": dut["link2"],
                "lag-port": {"lag": "lag0"}
            }] + net + lag
        }
    })


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env(edge_mappings=infamy.lag.edge_mappings)
        dut1 = env.attach("dut1", "mgmt")
        dut2 = env.attach("dut2", "mgmt")

    _, mon = env.ltop.xlate("host", "mon")
    with infamy.IsolatedMacVlan(mon) as ns:
        dm = {
            '1': ("dut1", dut1),
            '2': ("dut2", dut2)
        }
        lb = DumbLinkBreaker(env, dm, ns)
        ns.addip("192.168.2.1")

        with test.step("Set up LACP link aggregate, lag0, on dut1 and dut2"):
            parallel(lambda: dut_init(dut1, "lacp", None),
                     lambda: dut_init(dut2, "lacp", "192.168.2.42"))

        with test.step("Verify failure modes for lacp mode"):
            lb.fail_check("192.168.2.42")

        with test.step("Set up static link aggregate, lag0, on dut1 and dut2"):
            parallel(lambda: dut_init(dut1, "static", None),
                     lambda: dut_init(dut2, "static", "192.168.2.42"))

        with test.step("Verify failure modes for static mode"):
            lb.fail_check("192.168.2.42")

    test.succeed()
