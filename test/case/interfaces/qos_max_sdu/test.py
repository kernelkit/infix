#!/usr/bin/env python3
"""
QoS Maximum SDU

Set a queueMaxSDU on every traffic class of a bridge port with a gate
control list, and check that the port drops the frames longer than it
and forwards the rest, as IEEE 802.1Q-2022 8.6.8.4 specifies.

The DUT bridges two ports.  The port toward the listener runs a gate
control list with all gates open and a `queue-max-sdu` of 1000 bytes on
every class, counted like the MTU.  The talker pings the listener with
the don't-fragment bit set: a 928-byte IP packet must get through and a
1128-byte one must be dropped.
"""
import infamy
import infamy.qos as qos
from infamy.util import until

MAX_SDU = 1000
SMALL, LARGE = 900, 1100                     # ICMP payloads, 28 bytes below the IP size
SET_GATES = "ieee802-dot1q-sched:set-gate-states"


def bridge(td0, td1):
    return {"ietf-interfaces": {"interfaces": {"interface": [
        {"name": "br0", "type": "infix-if-type:bridge", "enabled": True},
        {"name": td0, "enabled": True, "infix-interfaces:bridge-port": {"bridge": "br0"}},
        {"name": td1, "enabled": True, "infix-interfaces:bridge-port": {"bridge": "br0"}},
    ]}}}


def schedule(td1, num_tc):
    return {"ietf-interfaces": {"interfaces": {"interface": [{
        "name": td1,
        "ieee802-dot1dc-sched-if:gate-parameter-table": {
            "gate-enabled": True,
            "admin-cycle-time": {"numerator": 1, "denominator": 1000},
            "admin-base-time": {"seconds": 0, "nanoseconds": 0},
            "admin-control-list": {"gate-control-entry": [
                {"index": 0, "operation-name": SET_GATES,
                 "gate-states-value": (1 << num_tc) - 1, "time-interval-value": 1_000_000}]},
            "queue-max-sdu-table": [{"traffic-class": tc, "queue-max-sdu": MAX_SDU}
                                    for tc in range(num_tc)],
        },
    }]}}}


def pings(ns, size):
    return ns.runsh(f"ping -c 3 -W 1 -M do -s {size} 192.168.20.2").returncode == 0


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
        if not target.has_model("ieee802-dot1dc-sched-if"):
            test.skip("the product has no ieee802-dot1dc-sched-if model")
        port.require_offload(test, "time-aware-shaper")
        if port.tx_queues < port.traffic_classes:
            test.skip(f"{td1} has {port.tx_queues} transmit queues, a schedule needs one per class")

    with test.step("Bridge the two ports"):
        target.put_config_dicts(bridge(td0, td1))

    with infamy.IsolatedMacVlan(hd0) as ns0, \
         infamy.IsolatedMacVlan(hd1) as ns1:

        with test.step("Set up the talker and listener namespaces"):
            ns0.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.1/24 dev iface")
            ns1.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.2/24 dev iface")
            ns0.must_reach("192.168.20.2")

        with test.step("Verify 928- and 1128-byte packets reach the listener without a max SDU"):
            assert pings(ns0, SMALL), "a 928-byte packet was lost"
            assert pings(ns0, LARGE), "a 1128-byte packet was lost"

        with test.step("Open all gates toward the listener, max SDU 1000 on every class"):
            target.put_config_dicts(schedule(td1, port.traffic_classes))
            until(lambda: (qos.root_qdisc(tgtssh, td1) or {}).get("kind") == "taprio")

        with test.step("Verify a 928-byte packet still reaches the listener"):
            assert pings(ns0, SMALL), "a 928-byte packet, below the max SDU, was lost"

        with test.step("Verify a 1128-byte packet is dropped"):
            assert not pings(ns0, LARGE), "a 1128-byte packet, above the max SDU, got through"

    test.succeed()
