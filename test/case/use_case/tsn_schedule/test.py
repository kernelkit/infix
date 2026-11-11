#!/usr/bin/env python3
"""
TSN Schedule

Run gPTP, a gate control list and a credit-based shaper together on one
bridge port, the usual TSN setup, and check that each traffic class
gets the share of the port its schedule gives it.

The egress port is an IEEE 802.1AS (gPTP) grandmaster, so the gates
run on its clock.  Its traffic classes follow the IEEE 802.1Q-2022
recommendations for a port carrying reserved streams.  The `ieee-sr`
preset, Table 34-1, puts SR class A, priority 3, in class 7, the
highest class.  Class 7 runs an IEEE 802.1Qav credit-based shaper
reserving 50 Mbps, the other classes are strict priority.

The IEEE 802.1Qbv gate control list repeats every 1 ms.  It opens a
protected window for class 3, scheduled traffic at priority 5, then a
window for class 7, the reserved stream, then one for the remaining
classes.  The cycle ends with a guard band, all gates closed for as
long as a maximum-size frame takes, so no frame from the last window
can overrun into the protected one.

.The gate control list on the egress port
image::schedule.svg[align=center, scaledwidth=90%]

The ingress port classifies three flows by DSCP with the default
`ietf` map, RFC 4594:

* EF, priority 5, to class 3, at 100 Mbps.  It fits in its window
  and must arrive intact.
* AF31, priority 3, to class 7, at 40 Mbps, below its reservation.
  It must also arrive intact.  The shaper earns credit only while the
  gate is open, so this holds only if the reservation is scaled up to
  make up for the time the gate is closed.
* CS1, priority 1, to class 0, at 400 Mbps, more than its window
  can carry.  It must be held to what fits in the window.  The test's
  own iperf3 control traffic, CS0, goes to class 1 in the same window.

Last, EF runs alone at 300 Mbps and must also be held to its
window.  Without the gates, strict priority would let it through at
line rate.  What a window carries is the number of whole frames that
fit in it at the port's speed, within 10 percent: about 184 Mbps for
a 200 µs window at 1 Gbps.
"""
import infamy
import infamy.ptp as ptp
import infamy.qos as qos
from infamy.util import until

EF, AF31, CS1 = 46 << 2, 26 << 2, 8 << 2     # TOS bytes of the three markings
SCHED, VIDEO = (qos.TABLE_34_1[8][prio] for prio in (5, 3))
SCHED_NS, VIDEO_NS = 200_000, 600_000        # gate intervals
CYCLE = 1_000_000
RESERVE = 50_000_000                         # bps, idle slope of the video class
SIZE = 1000                                  # UDP payload
OVERHEAD = 46 + qos.WIRE_OVERHEAD             # UDP, IP, Ethernet and FCS, then the wire
TOLERANCE = 0.1
CBS = "ieee802-dot1q-types:credit-based-shaper"
SET_GATES = "ieee802-dot1q-sched:set-gate-states"


def windows(speed):
    """(gate mask, interval) of each entry, the guard band last"""
    guard = qos.guard_band(speed)
    return [(1 << SCHED, SCHED_NS),
            (1 << VIDEO, VIDEO_NS),
            (0xff & ~(1 << SCHED | 1 << VIDEO), CYCLE - SCHED_NS - VIDEO_NS - guard),
            (0, guard)]


def gptp(port):
    return {"ieee1588-ptp-tt": {"ptp": {"instances": {"instance": [{
        "instance-index": 0,
        "default-ds": {
            "instance-type": "oc",
            "domain-number": 0,
            "priority1": 1,
            "priority2": 128,
            "infix-ptp:profile": "ieee802-dot1as",
        },
        "ports": {"port": [{"port-index": 1, "underlying-interface": port}]},
    }]}}}}


def bridge(td0, td1):
    return {"ietf-interfaces": {"interfaces": {"interface": [
        {"name": "br0", "type": "infix-if-type:bridge", "enabled": True},
        {
            "name": td0,
            "enabled": True,
            "infix-interfaces:bridge-port": {"bridge": "br0"},
            "infix-interfaces:qos": {"ingress": {"trust": "dscp"}}
        },
        {"name": td1, "enabled": True, "infix-interfaces:bridge-port": {"bridge": "br0"}},
    ]}}}


def schedule(td1, wins):
    return {"ietf-interfaces": {"interfaces": {"interface": [{
        "name": td1,
        "infix-interfaces:qos": {
            "egress": {"traffic-class-table": {"preset": "ieee-sr"},
                       "traffic-class": [{"id": VIDEO, "algorithm": CBS}]}
        },
        "ieee802-dot1dc-cbsa-if:cbsa": {
            "cbsa-parameter-table": [{"traffic-class": VIDEO, "admin-idle-slope": RESERVE}]
        },
        "ieee802-dot1dc-sched-if:gate-parameter-table": {
            "gate-enabled": True,
            "admin-cycle-time": {"numerator": 1, "denominator": 1_000_000_000 // CYCLE},
            "admin-base-time": {"seconds": 0, "nanoseconds": 0},
            "admin-control-list": {"gate-control-entry": [
                {"index": i, "operation-name": SET_GATES,
                 "gate-states-value": mask, "time-interval-value": interval}
                for i, (mask, interval) in enumerate(wins)]},
        },
    }]}}}


def window_rate(interval, speed):
    """Payload bit rate of the whole frames that fit in a window each cycle"""
    frames = interval * speed // 1_000_000_000 // ((SIZE + OVERHEAD) * 8)
    return frames * SIZE * 8 * 1_000_000_000 // CYCLE


def capped(flow, rate):
    flow.require_backlog()
    got = flow.throughput()
    print(f"{flow.name}: {got / 1e6:.2f} Mbps through, its window carries {rate / 1e6:.2f}")
    assert abs(got - rate) <= TOLERANCE * rate, \
        f"{flow.name}: {got / 1e6:.2f} Mbps through a window that carries {rate / 1e6:.2f}"


def intact(flow):
    assert flow.result["lost_percent"] < 1, \
        f"{flow.name} lost {flow.result['lost']} of {flow.result['offered']} datagrams"


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
        for model in ("ieee802-dot1dc-sched-if", "ieee802-dot1dc-cbsa-if"):
            if not target.has_model(model):
                test.skip(f"the product has no {model} model")
        port.require_classes(test, 8)
        port.require_offload(test, "time-aware-shaper")
        port.require_offload(test, "credit-based-shaper")
        if port.tx_queues < port.traffic_classes:
            test.skip(f"{td1} has {port.tx_queues} transmit queues, a schedule needs one per class")

    with test.step("Bridge the two ports, classify by DSCP on ingress"):
        target.put_config_dicts(bridge(td0, td1))

    with test.step("Start a gPTP grandmaster on the egress port"):
        target.put_config_dicts(gptp(td1))

    with test.step("Verify the DUT is its own grandmaster and the port transmits time"):
        until(lambda: ptp.is_own_gm(target) and ptp.is_time_transmitter(target), attempts=60)

    with test.step("Run the gate control list on the egress port, reserve 50 Mbps for class 7"):
        speed = int(target.get_iface(td1)["speed"])
        wins = windows(speed)
        target.put_config_dicts(schedule(td1, wins))
        until(lambda: {"taprio", "cbs"} <= {q.get("kind") for q in qos.qdiscs(tgtssh, td1)})
        qos.show_offload(port, "time-aware-shaper")
        qos.show_offload(port, "credit-based-shaper")
        sched = window_rate(SCHED_NS, speed)
        bulk = window_rate(wins[2][1], speed)

    with infamy.IsolatedMacVlan(hd0) as ns0, \
         infamy.IsolatedMacVlan(hd1) as ns1:

        with test.step("Set up the talker and listener namespaces"):
            ns0.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.1/24 dev iface")
            ns1.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.2/24 dev iface")
            ns0.must_reach("192.168.20.2")

        with test.step("EF at 100 Mbps, AF31 at 40 Mbps and CS1 at 400 Mbps"):
            ef, af31, cs1 = flows = [qos.Flow("EF", 5201, EF, 100_000_000, size=SIZE),
                                     qos.Flow("AF31", 5202, AF31, 40_000_000, size=SIZE),
                                     qos.Flow("CS1", 5203, CS1, 400_000_000, size=SIZE)]
            qos.run_flows(ns0, ns1, "192.168.20.2", flows)

        with test.step("Verify EF and AF31 arrived intact"):
            intact(ef)
            intact(af31)

        with test.step("Verify CS1 was held to what its window carries"):
            capped(cs1, bulk)

        with test.step("EF alone at 300 Mbps"):
            ef = qos.Flow("EF", 5201, EF, 300_000_000, size=SIZE)
            qos.run_flows(ns0, ns1, "192.168.20.2", [ef])

        with test.step("Verify EF was held to what its window carries"):
            capped(ef, sched)

    test.succeed()
