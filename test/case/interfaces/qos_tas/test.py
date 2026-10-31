#!/usr/bin/env python3
"""
QoS Time-Aware Shaper

Configure an IEEE 802.1Qbv gate control list on a port: a 100 µs cycle
with the top class alone for 20 µs, the top class and class 5 for
20 µs, and every other class for the remaining 60 µs.  The port must
run the list and report it back as the operational control list, with
the cycle and base time as given.

The gates exclude what cannot run under them: a port rate limit, since
the schedule takes the root, and weighted classes, since gated classes
are served strictly.  Both are rejected.  Skipped on a product without
the TSN modules, and a port with fewer transmit queues than traffic
classes rejects the schedule instead of running it, which is checked.
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
        _, port = env.ltop.xlate("target", "data")
        cap = infamy.capability.Port(target, port, tgtssh)
        if not target.has_model("ieee802-dot1dc-sched-if"):
            test.skip("the product has no time-aware shaper model")
        num_tc = cap.traffic_classes
        top = 1 << (num_tc - 1)
        rest = (1 << num_tc) - 1 - top - (1 << 5 if num_tc > 5 else 0)
        entries = [(top, 20000), (top | (1 << 5 if num_tc > 5 else 0), 20000), (rest, 60000)]
        txqs = int(tgtssh.runsh(f"ls -d /sys/class/net/{port}/queues/tx-* | wc -l").stdout)

    with test.step("Reject a gate control list together with a port rate limit"):
        must_reject(target, config(port, gates(port, entries),
                                   {"rate-limit": {"rate": 10_000_000}}),
                    "gates with rate-limit", "rate-limit and gate-enabled exclude each other")

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
