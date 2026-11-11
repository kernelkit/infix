#!/usr/bin/env python3
"""
QoS Mixed Selection

Strict priority and weighted sharing can run together on one port: the
strict classes are served first and the weighted classes divide what
is left.  This is the usual setup for a port that carries voice or
control traffic next to ordinary data.  This test fills a port with
three flows and checks both rules at once.

.What the three talkers offer and what the port must deliver
image::expected.svg[align=center, scaledwidth=90%]

Three talkers, marked EF, AF21 and CS1, send through the DUT's bridge
and out one port.  That port is slowed down so the test's traffic can
fill it: negotiated down to 100 Mbps, or shaped to 10 Mbps where
it has no PHY.  The port where the traffic enters puts EF in a strict
class above AF21 and CS1, which share the rest 2:1.

EF offers 40 percent of what the port can send and the other two
60 percent each, 160 percent in all.  EF must arrive intact.  AF21 and
CS1 must divide what EF leaves 2:1, within five points, and both must
be held back by more than a tenth of what they offer.

The weights are set as 60 and 30 percent.  Every class below a
weighted one must be weighted too, so the two classes below get the
remaining ten percent, five each, and carry no traffic.
"""
import infamy
import infamy.qos as qos
from infamy.util import until

EF, AF21, CS1 = 46 << 2, 18 << 2, 8 << 2    # TOS bytes of the three markings
STRICT, HIGH, LOW = 4, 3, 2                 # priorities, and classes, of the markings
FILLER = 5                                  # share of each class below, carrying nothing

SHARES = {HIGH: 60, LOW: 30}
SHARES.update({tc: FILLER for tc in range(LOW)})


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

    with test.step("Bridge the two ports, classify by DSCP on ingress, EF strict above AF21 and CS1 at 2:1"):
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
                                        "dscp-map": qos.dscp_map(**{"0": LOW, "46": STRICT,
                                                                     "18": HIGH, "8": LOW})}
                        }
                    },
                    {
                        "name": td1,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {"bridge": "br0"},
                        "infix-interfaces:qos": {
                            "egress": {"traffic-class": qos.traffic_classes(SHARES)}
                        }
                    },
                ]
            }
        }})
        quanta = [SHARES[tc] * qos.ETS_QUANTUM_UNIT for tc in sorted(SHARES, reverse=True)]
        until(lambda: qos.scheduler_matches(qos.scheduler(tgtssh, td1), num_tc,
                                            qos.TABLE_8_5[num_tc],
                                            strict=num_tc - len(SHARES), quanta=quanta))
        qos.show_offload(port)

    with test.step("Slow the egress port so its queues can fill"):
        drain = qos.slow_port(port, until)
        if not drain:
            print(f"{td1} can neither negotiate down nor be shaped, skipping")
            test.skip()
        print(f"{td1} drains {drain // 1_000_000} Mbps")

    with infamy.IsolatedMacVlan(hd0) as ns0, \
         infamy.IsolatedMacVlan(hd1) as ns1:

        with test.step("Set up the talker and listener namespaces"):
            ns0.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.1/24 dev iface")
            ns1.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.2/24 dev iface")
            ns0.must_reach("192.168.20.2")

        with test.step("EF at 40 percent, AF21 and CS1 at 60 percent each"):
            flows = [qos.Flow("EF", 5201, EF, int(drain * 0.4)),
                     qos.Flow("AF21", 5202, AF21, int(drain * 0.6)),
                     qos.Flow("CS1", 5203, CS1, int(drain * 0.6))]
            total = qos.run_flows(ns0, ns1, "192.168.20.2", flows)
            ef, af21, cs1 = flows
            print(f"shares: EF {ef.share(total):.1f}%, AF21 {af21.share(total):.1f}%, "
                  f"CS1 {cs1.share(total):.1f}%")

        with test.step("Verify EF arrived intact"):
            assert ef.result["lost_percent"] < 1, \
                f"EF lost {ef.result['lost']} of {ef.result['offered']} datagrams"

        with test.step("Verify AF21 and CS1 were both held back and split the rest 2:1"):
            for flow in (af21, cs1):
                flow.require_backlog()
            rest = af21.throughput() + cs1.throughput()
            af21_share = af21.share(rest)
            print(f"of what EF left: AF21 {af21_share:.1f}%, CS1 {100 - af21_share:.1f}%")
            assert abs(af21_share - 100 * 60 / 90) < 5, \
                f"AF21 got {af21_share:.1f}% of what EF left, configured 2:1"

    test.succeed()
