#!/usr/bin/env python3
"""
QoS Weighted Sharing

Two talkers, one marked EF and one marked CS1, bridged through the DUT
and out one egress port that cannot carry both.  The port is negotiated
down to 100 Mbit/s where its PHY allows, or shaped to 10 Mbit/s
where it has no PHY.  The ingress port classifies EF to priority 3 and
CS1 to priority 2, classes 3 and 2 in the default table, both running
enhanced transmission selection; the two classes below hold five
percent each and carry nothing, since strict classes must sit above the
weighted ones.  Each talker offers 80 percent of what the port drains,
so both classes always have a backlog and the scheduler decides the
split.

Two measurements:

 - equal weights, 45 and 45 percent, must give an even split.  This is
   the baseline, and fails on its own if the shares are applied to the
   wrong bands
 - 2:1, configured as 60 and 30 percent, must move the split to match

Each flow's share of what the port delivers in the middle seconds of the
flows must land within five points of its configured share of the two,
and both flows must be held back by more than a tenth of what they offer
in both measurements, which shows the port was full.  The
test's own iperf3 control traffic, DSCP 0, rides with CS1 in the lowest
loaded class, so a talker's end-of-test message leaves the port behind
its last datagram.
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
        port.require_classes(test, 3)
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
