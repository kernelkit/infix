#!/usr/bin/env python3
"""
QoS Rate Limit

One talker, bridged through the DUT and out an egress port with a
10 Mbit/s rate limit, offering three times that.  What arrives at the
listener must be the limit, within a fifth, whatever the priority of the
flow: once marked EF, in a strict class near the top, once marked CS1,
in the lowest class.  Then both at once, each offering one and a half
times the limit, must still add up to the limit.

A limit that lets more through is not a limit, and a limit that only
bites on some queues is a scheduler fault dressed up as one.  The test's
own iperf3 control traffic, DSCP 0, is classified to priority 7 at the
ingress port.  Skipped on a switch fabric whose driver does not offload
the rate limit, where forwarded frames never meet the bucket.
"""
import infamy
import infamy.qos as qos
from infamy.util import until

LIMIT = 10_000_000             # bit/s, Layer 2
EF, CS1 = 46 << 2, 8 << 2      # TOS bytes of the two markings
SIZE = 1000                    # UDP payload, 1042 bytes on the wire
TOLERANCE = 0.2


def check(what, throughput):
    """throughput is payload bit/s; the limit counts the 42 header bytes too"""
    expect = LIMIT * SIZE / (SIZE + 42)
    print(f"{what}: {throughput / 1e6:.2f} Mbit/s through, limit lets {expect / 1e6:.2f}")
    assert abs(throughput - expect) <= TOLERANCE * expect, \
        f"{what}: {throughput / 1e6:.2f} Mbit/s through a {LIMIT / 1e6:.0f} Mbit/s limit"


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        _, td0 = env.ltop.xlate("target", "data1")
        _, td1 = env.ltop.xlate("target", "data2")
        _, hd0 = env.ltop.xlate("host", "data1")
        _, hd1 = env.ltop.xlate("host", "data2")

    with test.step("Bridge the two ports, trust DSCP on ingress, rate limit the egress port"):
        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {"name": "br0", "type": "infix-if-type:bridge", "enabled": True},
                    {
                        "name": td0,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {"bridge": "br0"},
                        "infix-interfaces:qos": {
                            "ingress": {"trust": "dscp",
                                        "dscp-map": qos.dscp_map(**{"0": 7, "46": 5, "8": 1})}
                        }
                    },
                    {
                        "name": td1,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {"bridge": "br0"},
                        "infix-interfaces:qos": {
                            "egress": {"rate-limit": {"rate": LIMIT}}
                        }
                    },
                ]
            }
        }})
        until(lambda: (qos.root_qdisc(tgtssh, td1) or {}).get("kind") == "tbf")
        qos.show_shaper(tgtssh, td1)

        uevent = tgtssh.runsh(f"cat /sys/class/net/{td1}/uevent").stdout
        if "DEVTYPE=dsa" in uevent.split() and "rate-limit" not in qos.offload(target, td1):
            print("switch fabric forwards past a rate limit its driver does not offload, skipping")
            test.skip()

    with infamy.IsolatedMacVlan(hd0) as ns0, \
         infamy.IsolatedMacVlan(hd1) as ns1:

        with test.step("Set up the talker and listener namespaces"):
            ns0.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.1/24 dev iface")
            ns1.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.2/24 dev iface")
            ns0.must_reach("192.168.20.2")

        with test.step("One EF flow at three times the limit, expect the limit through"):
            flow = qos.Flow("EF", 5201, EF, 3 * LIMIT, size=SIZE)
            qos.run_flows(ns0, ns1, "192.168.20.2", [flow])
            check("EF alone", flow.throughput())

        with test.step("One CS1 flow at three times the limit, expect the limit through"):
            flow = qos.Flow("CS1", 5202, CS1, 3 * LIMIT, size=SIZE)
            qos.run_flows(ns0, ns1, "192.168.20.2", [flow])
            check("CS1 alone", flow.throughput())

        with test.step("Both at one and a half times the limit, expect the limit through in total"):
            flows = [qos.Flow("EF", 5201, EF, int(1.5 * LIMIT), size=SIZE),
                     qos.Flow("CS1", 5202, CS1, int(1.5 * LIMIT), size=SIZE)]
            total = qos.run_flows(ns0, ns1, "192.168.20.2", flows)
            print(f"shares: EF {flows[0].share(total):.1f}%, CS1 {flows[1].share(total):.1f}%")
            check("EF and CS1 together", sum(f.throughput() for f in flows))

    test.succeed()
