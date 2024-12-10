#!/usr/bin/env python3
r"""Bridge STP Basic

Verify that a fully connected mesh of 4 DUTs is pruned to a spanning
tree.

Since the mesh contains 3 redundant paths, can infer that a spanning
tree has been created if all host interfaces can reach each other
while exactly three links are in the blocking state.

"""
import infamy
from infamy.util import parallel, until

def addbr(dut):
    ip = {
        "A": "10.0.0.101",
        "B": "10.0.0.102",
        "C": "10.0.0.103",
        "D": "10.0.0.104"
    }[dut.name]

    brports = [
        {
            "name": dut[n],
            "infix-interfaces:bridge-port": {
                "bridge": "br0",
            }
        } for n in ("a", "b", "c", "d", "h") if n != dut.name.lower()
    ]

    dut.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                        "bridge": {
                            "stp": {},
                        },
                        "ipv4": {
                            "address": [
                                {
                                    "ip": ip,
                                    "prefix-length": 24,
                                }
                            ]
                        },
                    }
                ] + brports,
            }
        }
    })

def num_blocking(dut):
    num = 0
    for iface in dut.get_data("/ietf-interfaces:interfaces")["interfaces"]["interface"]:
        if iface.get("bridge-port", {}).get("stp-state") == "blocking":
            num += 1

    return num

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        a, b, c, d = parallel(lambda: env.attach("A"), lambda: env.attach("B"),
                              lambda: env.attach("C"), lambda: env.attach("D"))

        host = { p: env.ltop.xlate("host", p)[1] for p in ("a", "b", "c", "d") }

    with test.step("Configure a bridge with spanning tree eneabled on dut a, b, c, and d"):
        parallel(lambda: addbr(a), lambda: addbr(b), lambda: addbr(c), lambda: addbr(d))

    with test.step("Add an IP address to each host interface in the 10.0.0.0/24 subnet"):
        ns = { p: infamy.IsolatedMacVlan(host[p]).start() for p in ("a", "b", "c", "d") }
        parallel(lambda: ns["a"].addip("10.0.0.1"), lambda: ns["b"].addip("10.0.0.2"),
                 lambda: ns["c"].addip("10.0.0.3"), lambda: ns["d"].addip("10.0.0.4"))

    with test.step("Verify that exactly three links are blocking"):
        until(lambda: sum(map(num_blocking, (a, b, c, d))) == 3, 60)

    with test.step("Verify that host:a can reach host:{b,c,d}"):
        parallel(lambda: ns["a"].must_reach("10.0.0.2"),
                 lambda: ns["a"].must_reach("10.0.0.3"),
                 lambda: ns["a"].must_reach("10.0.0.4"))

    test.succeed()
