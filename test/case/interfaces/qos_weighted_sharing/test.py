#!/usr/bin/env python3
"""
QoS Weighted Sharing

Two talkers, one marked EF and one marked CS1, bridged through the DUT
and out one egress port that cannot carry both.  The port is negotiated
down to 100 Mbit/s where its PHY allows, or rate limited to 10 Mbit/s
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

Each flow's share of the delivered datagrams must land within five
points of its configured share of the two, and both flows must lose more
than a tenth of their frames in both measurements; a port that was never
full proves nothing.  The test's own iperf3 control traffic, DSCP 0, is
classified to priority 7.  Skipped on a switch fabric whose scheduler is
not offloaded, where forwarded frames never meet the configured
algorithm.
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
        _, td0 = env.ltop.xlate("target", "data1")
        _, td1 = env.ltop.xlate("target", "data2")
        _, hd0 = env.ltop.xlate("host", "data1")
        _, hd1 = env.ltop.xlate("host", "data2")

        num_tc = qos.num_classes(target, td1)
        print(f"{td1}: {num_tc} traffic classes")
        if num_tc < 3:
            print("no class left for the control traffic above two weighted ones, skipping")
            test.skip()

        uevent = tgtssh.runsh(f"cat /sys/class/net/{td1}/uevent").stdout
        dsa = "DEVTYPE=dsa" in uevent.split()
        if dsa and "transmission-selection" not in qos.offload(target, td1):
            print("switch fabric forwards past a scheduler its driver does not offload, skipping")
            test.skip()

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
                                        "dscp-map": qos.dscp_map(**{"0": 7, "46": HIGH, "8": LOW})}
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
            qos.show_offload(target, tgtssh, td1, dsa)
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
                assert flow.result["lost_percent"] > 10, \
                    f"{flow.name} lost {flow.result['lost_percent']:.1f}%, the port was never full"
            assert abs(ef.share(total) - expect) < 5, \
                f"EF got {ef.share(total):.1f}% of the port, configured {expect:.0f}%"

        with test.step("Equal weights, both flows at 80 percent, expect an even split"):
            measure(45, 45)

        with test.step("Weights 2:1, both flows at 80 percent, expect a 67:33 split"):
            measure(60, 30)

    test.succeed()
