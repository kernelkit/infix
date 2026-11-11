#!/usr/bin/env python3
"""
QoS Weighted Sharing

Weighted sharing divides a full port between traffic classes in the
proportions configured, so every class keeps its share however much
the others offer.  This test fills a port with two flows and checks
that the split follows the weights.

.What the two talkers offer and what the port must deliver
image::expected.svg[align=center, scaledwidth=90%]

Two talkers, one marked EF and one marked CS1, send through the DUT's
bridge and out one port.  That port is slowed down so the test's
traffic can fill it: negotiated down to 100 Mbit/s, or shaped to
10 Mbit/s where it has no PHY.  The port where the traffic enters puts
the two flows in two traffic classes, both running enhanced
transmission selection, the weighted algorithm of IEEE 802.1Q.  Each
talker offers 80 percent of what the port can send, so both classes
always have frames waiting.

Two measurements:

 - equal weights must give an even split.  This is the baseline, and
   fails on its own if the weights land on the wrong classes
 - weights 2:1 must give one flow two thirds and the other one third

Each flow's share of what the port delivers must be within five points
of its weight.  Both flows must also be held back by more than a tenth
of what they offer, which shows the port was full.

The weights are set as 45 and 45 percent, then 60 and 30.  Every class
below a weighted one must be weighted too, so the two classes below
get the remaining ten percent, five each, and carry no traffic.
"""
import infamy
import infamy.qos as qos
from infamy.util import until

EF, CS1 = 46 << 2, 8 << 2      # TOS bytes of the two markings
HIGH, LOW = 3, 2               # priorities, and classes, of the two markings
FILLER = 5                     # share of each class below the two, carrying nothing


def layout(high, low):
    """The weighted classes: the two under test and the fillers below"""
    shares = {HIGH: high, LOW: low}
    shares.update({tc: FILLER for tc in range(LOW)})
    return qos.traffic_classes(shares)


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
        num_tc = port.traffic_classes
        print(f"{td1}: {num_tc} traffic classes")
        port.require_classes(test, 7)
        port.require_offload(test, "transmission-selection")

    with test.step("Bridge the two ports, classify by DSCP on ingress, EF and CS1 in two weighted classes"):
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
                        "infix-interfaces:qos": {
                            "egress": {"traffic-class": layout(45, 45)}
                        }
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

        def apply(high, low):
            target.put_config_dicts({"ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": td1,
                        "infix-interfaces:qos": {"egress": {"traffic-class": layout(high, low)}}
                    }]
                }
            }})
            quanta = [share * qos.ETS_QUANTUM_UNIT
                      for share in [high, low] + [FILLER] * LOW]
            until(lambda: qos.scheduler_matches(qos.scheduler(tgtssh, td1), num_tc,
                                                qos.TABLE_8_5[num_tc],
                                                strict=num_tc - 2 - LOW, quanta=quanta))
            qos.show_offload(port)
            ns0.must_reach("192.168.20.2")

        def measure(high, low):
            apply(high, low)
            flows = [qos.Flow("EF", 5201, EF, int(drain * 0.8)),
                     qos.Flow("CS1", 5202, CS1, int(drain * 0.8))]
            total = qos.run_flows(ns0, ns1, "192.168.20.2", flows)
            ef, cs1 = flows
            expect = 100.0 * high / (high + low)
            print(f"shares: EF {ef.share(total):.1f}%, CS1 {cs1.share(total):.1f}%, "
                  f"configured {high}:{low}")
            for flow in flows:
                flow.require_backlog()
            assert abs(ef.share(total) - expect) < 5, \
                f"EF got {ef.share(total):.1f}% of the port, configured {expect:.0f}%"

        with test.step("Equal weights, both flows at 80 percent, expect an even split"):
            measure(45, 45)

        with test.step("Weights 2:1, both flows at 80 percent, expect a 67:33 split"):
            measure(60, 30)

    test.succeed()
