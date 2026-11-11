#!/usr/bin/env python3
"""
QoS Strict Priority

When a port has more to send than it can carry, strict priority decides
who goes first: a higher traffic class is always served before a lower
one.  This test fills a port with two flows and checks that the
important one gets through untouched, also when that leaves nothing
for the other.

.What the two talkers offer and what the port must deliver
image::expected.svg[align=center, scaledwidth=90%]

Two talkers, one marked EF and one marked CS1, send through the DUT's
bridge and out one port.  That port is slowed down so the test's
traffic can fill it: negotiated down to 100 Mbit/s, or shaped to
10 Mbit/s where it has no PHY.  The port where the traffic enters
decides the priority of each flow from its DSCP marking.

Three measurements:

 - baseline: both markings get the same priority and each talker offers
   80 percent of what the port can send.  They share one queue and
   CS1 must keep at least a tenth of the port, so whatever the next
   two measurements show comes from the priorities
 - split: EF gets the higher priority, 80 percent each.  EF must
   arrive without loss and CS1 gets what is left
 - starve: EF offers 120 percent of the port.  CS1 must get less than
   5 percent

In every measurement the port must hold back more than a tenth of what
is offered, which shows it was full.
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
        td0 = target["data1"]
        td1 = target["data2"]
        hd0 = env["host"]["data1"]
        hd1 = env["host"]["data2"]

        port = infamy.capability.Port(target, td1, tgtssh)
        infamy.capability.Port(target, td0, tgtssh).require_offload(test, "classification")
        print(f"{td1}: {port.traffic_classes} traffic classes, "
              f"EF at priority {HIGH}, CS1 at priority {LOW}")
        port.require_offload(test, "transmission-selection")

    with test.step("Bridge the two ports, classify by DSCP on ingress, control traffic with CS1"):
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
                                        "dscp-map": qos.dscp_map(**{"0": LOW, "46": HIGH, "8": LOW})}
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
        drain = qos.slow_port(port, until)
        if not drain:
            print(f"{td1} can neither negotiate down nor be shaped, skipping")
            test.skip()
        print(f"{td1} drains {drain // 1_000_000} Mbit/s")

    with infamy.IsolatedMacVlan(hd0) as ns0, \
         infamy.IsolatedMacVlan(hd1) as ns1:

        with test.step("Set up the talker and listener namespaces"):
            ns0.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.1/24 dev iface")
            ns1.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.2/24 dev iface")
            ns0.must_reach("192.168.20.2")

        def apply(ef_prio, cs1_prio):
            """Classify the two markings, then let the setup settle"""
            target.put_config_dicts({"ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": td0,
                        "infix-interfaces:qos": {
                            "ingress": {"dscp-map": qos.dscp_map(**{"0": LOW, "46": ef_prio,
                                                                     "8": cs1_prio})}
                        }
                    }]
                }
            }})
            until(lambda: qos.dscp_prio(tgtssh, td0, 46) == ef_prio and
                  qos.dscp_prio(tgtssh, td0, 8) == cs1_prio)
            qos.show_offload(port)
            ns0.must_reach("192.168.20.2")

        def measure(high_rate, low_rate=0.8):
            flows = [qos.Flow("EF", 5201, EF, int(drain * high_rate)),
                     qos.Flow("CS1", 5202, CS1, int(drain * low_rate))]
            total = qos.run_flows(ns0, ns1, "192.168.20.2", flows)
            high, low = flows
            print(f"shares: EF {high.share(total):.1f}%, CS1 {low.share(total):.1f}%")
            return high, low, total

        with test.step("Baseline: both flows at one priority, 80 percent each, expect neither starved"):
            apply(LOW, LOW)
            high, low, total = measure(0.8)
            assert total < 0.9 * (high.rate + low.rate), \
                f"{total / 1e6:.2f} Mbit/s through, the port was never full"
            assert low.share(total) >= 10, f"CS1 got {low.share(total):.1f}% with no priority in play"

        with test.step("Split: EF strict above CS1, 80 percent each, expect EF without loss"):
            apply(HIGH, LOW)
            high, low, total = measure(0.8)
            low.require_backlog()
            # EF offers less than the port drains, so it passes intact; a
            # stray datagram at the iperf3 startup edge is not congestion
            assert high.result["lost_percent"] < 1, \
                f"EF lost {high.result['lost']} of {high.result['offered']} datagrams"

        with test.step("Starve: EF at 120 percent of the port, expect CS1 below 5 percent"):
            high, low, total = measure(1.2)
            low.require_backlog()
            assert low.share(total) < 5, f"CS1 got {low.share(total):.1f}% of the port"

    test.succeed()
