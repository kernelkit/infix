#!/usr/bin/env python3
"""
QoS Strict Priority

Two talkers, one marked EF and one CS1, bridged through the DUT and out
one egress port that cannot carry both.  The port is negotiated down to
100 Mbit/s where its PHY allows, or rate limited to 10 Mbit/s where it
has no PHY, and each talker offers 80 percent of what the port drains,
so the queues fill and the scheduler decides who gets through.

Classification at the ingress port decides the priority, and with it
the queue, all the way to the listener: on one chip, across a cascade,
or through a network of switches.  So the flows are steered with the
DSCP map on the ingress port and the class table is left at its
default.  Three measurements, in this order:

 - baseline: both markings classified to the same priority share one
   queue, and the low flow keeps at least a tenth of it; one FIFO under
   tail drop is not fair, but it favours nobody by marking
 - split: EF classified to priority 5, strict above CS1 at priority 2,
   arrives without loss and CS1 takes what is left
 - starve: EF raised to 120 percent of the port on its own leaves CS1
   below 5 percent of the served datagrams

The low flow must lose more than a tenth of its frames in every
measurement; a port that was never full proves nothing.  The test's own
iperf3 control traffic, DSCP 0, is classified to priority 7 so the
contest never cuts it off.  Skipped on a switch fabric whose scheduler
is not offloaded, where forwarded frames never meet the configured
algorithm.
"""
import infamy
import infamy.qos as qos
from infamy.util import until

EF, CS1 = 46 << 2, 8 << 2      # TOS bytes of the two markings
HIGH, LOW = 5, 2               # priorities the two markings are classified to


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        _, td0 = env.ltop.xlate("target", "data1")
        _, td1 = env.ltop.xlate("target", "data2")
        _, hd0 = env.ltop.xlate("host", "data1")
        _, hd1 = env.ltop.xlate("host", "data2")

        num_tc = qos.num_classes(target, td1)
        print(f"{td1}: {num_tc} traffic classes, EF at priority {HIGH}, CS1 at priority {LOW}")

        uevent = tgtssh.runsh(f"cat /sys/class/net/{td1}/uevent").stdout
        dsa = "DEVTYPE=dsa" in uevent.split()
        if dsa and "transmission-selection" not in qos.offload(target, td1):
            print("switch fabric forwards past a scheduler its driver does not offload, skipping")
            test.skip()

    with test.step("Bridge the two ports, classify by DSCP on ingress, control traffic on top"):
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
                                        "dscp-map": qos.dscp_map(**{"0": 7, "46": HIGH, "8": LOW})}
                        }
                    },
                    {
                        "name": td1,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {"bridge": "br0"},
                    },
                ]
            }
        }})

    with test.step("Slow the egress port so its queues can fill"):
        drain = qos.slow_port(target, tgtssh, td1, until)
        if not drain:
            print(f"{td1} can neither negotiate down nor be rate limited, skipping")
            test.skip()
        print(f"{td1} drains {drain // 1_000_000} Mbit/s")

    with infamy.IsolatedMacVlan(hd0) as ns0, \
         infamy.IsolatedMacVlan(hd1) as ns1:

        with test.step("Set up the talker and listener namespaces"):
            ns0.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.1/24 dev iface")
            ns1.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.2/24 dev iface")
            ns0.must_reach("192.168.20.2")

        def apply(ef_prio, cs1_prio):
            """Classify the two markings, then let the rendering settle"""
            target.put_config_dicts({"ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": td0,
                        "infix-interfaces:qos": {
                            "ingress": {"dscp-map": qos.dscp_map(**{"0": 7, "46": ef_prio,
                                                                     "8": cs1_prio})}
                        }
                    }]
                }
            }})
            until(lambda: qos.dscp_prio(tgtssh, td0, 46) == ef_prio and
                  qos.dscp_prio(tgtssh, td0, 8) == cs1_prio)
            qos.show_offload(target, tgtssh, td1, dsa)
            ns0.must_reach("192.168.20.2")

        def measure(high_rate, low_rate=0.8):
            flows = [qos.Flow("EF", 5201, EF, int(drain * high_rate)),
                     qos.Flow("CS1", 5202, CS1, int(drain * low_rate))]
            total = qos.run_flows(ns0, ns1, "192.168.20.2", flows)
            high, low = flows
            print(f"shares: EF {high.share(total):.1f}%, CS1 {low.share(total):.1f}%")
            assert low.result["lost_percent"] > 10, \
                f"the low priority flow lost {low.result['lost_percent']:.1f}%, the port was never full"
            return high, low, total

        with test.step("Baseline: both flows at one priority, 80 percent each, expect neither starved"):
            apply(LOW, LOW)
            high, low, total = measure(0.8)
            assert low.share(total) >= 10, f"CS1 got {low.share(total):.1f}% with no priority in play"

        with test.step("Split: EF strict above CS1, 80 percent each, expect EF without loss"):
            apply(HIGH, LOW)
            high, low, total = measure(0.8)
            # EF offers less than the port drains, so it passes intact; a
            # stray datagram at the iperf3 startup edge is not congestion
            assert high.result["lost_percent"] < 1, \
                f"EF lost {high.result['lost']} of {high.result['offered']} datagrams"

        with test.step("Starve: EF at 120 percent of the port, expect CS1 below 5 percent"):
            high, low, total = measure(1.2)
            assert low.share(total) < 5, f"CS1 got {low.share(total):.1f}% of the port"

    test.succeed()
