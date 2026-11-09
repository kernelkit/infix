#!/usr/bin/env python3
"""
QoS Time-Aware Shaper

A time-aware shaper opens and closes a gate in front of each traffic
class on a fixed, repeating timetable, the gate control list of IEEE
802.1Qbv, so each kind of traffic has the port to itself at set times.
This test configures a list on a port and checks that the port runs it
and reports it back.  It sends no traffic: the TSN tests check the
timing on the wire.

.The gate control list of the test
image::schedule.svg[align=center, scaledwidth=90%]

The list repeats every 100 µs: the top class alone for 20 µs, the top
class and the class two below it for 20 µs, and every other class for
the remaining 60 µs.  The port must run the list and report it back as
the operational control list, with the cycle and base time as given.

Settings that cannot run under gates must be rejected: a port egress
shaper, a weighted class, since gated classes are served strictly, an
empty list, and a list on a port with fewer transmit queues than
traffic classes.
"""
import infamy
import infamy.qos as qos
from infamy.util import until

ETS = "ieee802-dot1q-types:enhanced-transmission-selection"
SET_GATES = "ieee802-dot1q-sched:set-gate-states"


def gates(port, entries, enabled=True):
    return {
        "gate-enabled": enabled,
        "admin-cycle-time": {"numerator": 1, "denominator": 10000},
        "admin-base-time": {"seconds": 0, "nanoseconds": 200},
        "admin-control-list": {"gate-control-entry": [
            {"index": i, "operation-name": SET_GATES,
             "gate-states-value": mask, "time-interval-value": interval}
            for i, (mask, interval) in enumerate(entries)]},
    }


def config(port, table, qos=None):
    iface = {"name": port, "enabled": True,
             "ieee802-dot1dc-sched-if:gate-parameter-table": table}
    if qos:
        iface["infix-interfaces:qos"] = qos
    return {"ietf-interfaces": {"interfaces": {"interface": [iface]}}}


def must_reject(target, cfg, what, reason):
    qos.must_reject(what, lambda: target.put_config_dicts(cfg), reason)


def oper_table(target, port):
    data = target.get_data(f"/ietf-interfaces:interfaces/interface[name='{port}']")
    iface = next(iter(data["interfaces"]["interface"]), {})
    return (iface.get("ieee802-dot1dc-sched-if:gate-parameter-table")
            or iface.get("gate-parameter-table") or {})


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        port = target["data"]
        cap = infamy.capability.Port(target, port, tgtssh)
        if not target.has_model("ieee802-dot1dc-sched-if"):
            test.skip("the product has no time-aware shaper model")
        cap.require_offload(test, "time-aware-shaper")
        num_tc = cap.traffic_classes
        top = 1 << (num_tc - 1)
        mid = 1 << (num_tc - 3) if num_tc > 2 else 0
        rest = (1 << num_tc) - 1 - top - mid
        entries = [(top, 20000), (top | mid, 20000), (rest, 60000)]
        txqs = cap.tx_queues

    with test.step("Reject a gate control list together with a port egress shaper"):
        must_reject(target, config(port, gates(port, entries),
                                   {"egress-shaper": {"rate": 10_000_000}}),
                    "gates with egress-shaper", "egress-shaper and gate-enabled exclude each other")

    with test.step("Reject a gate control list together with a weighted class"):
        must_reject(target, config(port, gates(port, entries),
                                   {"egress": {"traffic-class": [{"id": 0, "algorithm": ETS, "bandwidth": 100}]}}),
                    "gates with a weighted class", "cannot be gated")

    with test.step("Reject an empty gate control list"):
        must_reject(target, config(port, gates(port, [])),
                    "gates with an empty list", "empty admin-control-list")

    if txqs < num_tc:
        with test.step("Reject the schedule on a port with fewer transmit queues than classes"):
            must_reject(target, config(port, gates(port, entries)),
                        f"gates on a port with {txqs} transmit queues",
                        "needs a transmit queue per traffic class")
        test.succeed()

    with test.step("Enable the gate control list on the port"):
        target.put_config_dicts(config(port, gates(port, entries)))
        until(lambda: (qos.root_qdisc(tgtssh, port) or {}).get("kind") == "taprio")
        qos.show_offload(cap, "time-aware-shaper")

    with test.step("Verify the port reports the list, cycle and base time it runs"):
        def runs_list():
            table = oper_table(target, port)
            entries_run = [(e["gate-states-value"], e["time-interval-value"])
                           for e in table.get("oper-control-list", {}).get("gate-control-entry", [])]
            return (entries_run == entries
                    and table.get("oper-cycle-time") == {"numerator": 1, "denominator": 10000}
                    and table.get("oper-base-time", {}).get("nanoseconds") == 200)
        until(runs_list)

    test.succeed()
