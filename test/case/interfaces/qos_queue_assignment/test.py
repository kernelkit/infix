#!/usr/bin/env python3
"""
QoS Queue Assignment

Every frame has a priority, and a port's traffic class table says
which queue each priority uses.  This test checks that frames end up
in the queue the table names, for the `ieee` preset, the `ieee-sr`
preset and a custom table.

A hundred datagrams are sent at each priority, routed through the DUT
to the listener, and the egress port's per-class counters are read
before and after.  The class the table names must grow by at least a
hundred, and no other class by anywhere near it.

Frames enter tagged, on a VLAN interface that takes the priority
straight from the PCP, so the table on the egress port is all that is
under test.  The rate is low, so no queue fills.  mDNS is switched off
on the DUT, since its announcements would add over a hundred frames at
priority 0.  What remains of the system's own traffic is a handful of
frames.
"""
import infamy
import infamy.qos as qos
from infamy.util import until

PRIORITIES = range(8)
COUNT = 100     # per priority; the DUT's own chatter is a few frames

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        td0 = target["data1"]
        td1 = target["data2"]
        hd0 = env["host"]["data1"]
        hd1 = env["host"]["data2"]

        num_tc = infamy.capability.Port(target, td1).traffic_classes
        print(f"{td1}: {num_tc} traffic classes")
        tables = {
            "ieee": ({"preset": "ieee"}, qos.TABLE_8_5[num_tc]),
            "ieee-sr": ({"preset": "ieee-sr"}, qos.TABLE_34_1[num_tc]),
        }
        # Table 8-5 with the two lowest classes swapped, custom on any class count
        custom = [1 if tc == 0 else 0 if tc == 1 else tc for tc in qos.TABLE_8_5[num_tc]]
        tables["custom"] = ({f"priority{p}": tc for p, tc in enumerate(custom)}, custom)

    with test.step("Configure a routed path, tagged ingress with priority from PCP, egress under test"):
        target.put_config_dicts({"infix-services": {"mdns": {"enabled": False}}})
        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {"name": td0, "enabled": True},
                    {
                        "name": f"{td0}.10",
                        "type": "infix-if-type:vlan",
                        "vlan": {
                            "id": 10,
                            "lower-layer-if": td0,
                            "ingress-qos": {"priority": "from-pcp"},
                        },
                        "ipv4": {
                            "forwarding": True,
                            "address": [{"ip": "192.168.10.1", "prefix-length": 24}]
                        }
                    },
                    {
                        "name": td1,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{"ip": "192.168.11.1", "prefix-length": 24}]
                        }
                    },
                ]
            }
        }})

    with infamy.IsolatedMacVlan(hd0) as ns0, \
         infamy.IsolatedMacVlan(hd1) as ns1:

        with test.step("Set up the talker on VLAN 10 and the listener, resolve neighbours"):
            ns0.runsh("""
            set -ex
            ip link set iface up
            ip link add dev vlan10 link iface up type vlan id 10
            ip addr add 192.168.10.2/24 dev vlan10
            """)
            ns0.addroute("default", "192.168.10.1")
            ns1.runsh("""
            set -ex
            ip link set iface up
            ip addr add 192.168.11.2/24 dev iface
            """)
            ns1.addroute("default", "192.168.11.1")
            ns0.must_reach("192.168.11.2")
            dut_mac = qos.neighbour_mac(ns0, "192.168.10.1")
            assert dut_mac, "DUT MAC not resolved"
            print(f"DUT {td0}.10 is {dut_mac}")

        cleared = []
        for name, (table, prio_map) in tables.items():
            with test.step(f"Apply the {name} traffic class table on the egress port"):
                # Preset and custom leaves are a choice, so clear the previous table
                target.put_config_dicts({"ietf-interfaces": {
                    "interfaces": {
                        "interface": [{
                            "name": td1,
                            "infix-interfaces:qos": {"egress": {"traffic-class-table": table}}
                        }]
                    }
                }}, delete=cleared)
                cleared = [qos.xpath(td1, "/egress/traffic-class-table")]
                until(lambda: qos.scheduler_matches(qos.scheduler(tgtssh, td1), num_tc, prio_map))
                ns0.must_reach("192.168.11.2")

            with test.step(f"Send {COUNT} frames at each priority, verify the class each reached"):
                for prio in PRIORITIES:
                    before = qos.class_stats(tgtssh, td1, num_tc)
                    qos.mausezahn(ns0, "iface", "192.168.10.2", "192.168.11.2", dut_mac,
                                  count=COUNT, vid=10, pcp=prio)

                    def landed():
                        delta = qos.stats_delta(before, qos.class_stats(tgtssh, td1, num_tc))
                        return delta.get(prio_map[prio], {}).get("packets", 0) >= COUNT

                    until(landed, attempts=20)
                    delta = qos.stats_delta(before, qos.class_stats(tgtssh, td1, num_tc))
                    got = {tc: d["packets"] for tc, d in delta.items() if d["packets"]}
                    print(f"{name}: priority {prio} -> class {prio_map[prio]}: {got}")

                    for tc, d in delta.items():
                        if tc == prio_map[prio]:
                            assert d["packets"] >= COUNT, \
                                f"priority {prio}: class {tc} got {d['packets']}, expected at least {COUNT}"
                        else:
                            assert d["packets"] < COUNT // 2, \
                                f"priority {prio}: class {tc} got {d['packets']} frames it should not have"

    test.succeed()
