#!/usr/bin/env python3
"""
TSN Gate Timing

Check frame by frame that a bridge port's gate control list sends each
traffic class only inside its window, timed by the gPTP grandmaster's
clock.

The DUT bridges a talker and a listener and is the IEEE 802.1AS (gPTP)
grandmaster on both ports, so all three clocks share its time.  The
port toward the listener uses the `ieee-sr` traffic class preset and
runs a 1 ms IEEE 802.1Qbv gate control list, the same as in the TSN
Schedule test.  Class 3, scheduled traffic, gets 0 to 200 µs and
class 7, SR class A, 200 to 800 µs.  The remaining classes get the
rest of the cycle up to a guard band, 12.3 µs at 1 Gbps, when all
gates are closed.

.The gate control list on the DUT's port toward the listener
image::schedule.svg[align=center, scaledwidth=90%]

The talker sends `ptping` probes, UDP datagrams timestamped in
hardware when they leave the talker and when they reach the listener.
There are three streams, marked EF, AF31 and CS1, which the DUT
classifies with the default `ietf` DSCP map to priority 5, 3 and 1,
and so to class 3, 7 and 0.  Each stream sends one probe every
1.003 ms, so its probes reach the DUT at every point of the cycle in
turn.  A probe that reaches the DUT while its gate is closed must wait
for the gate to open.  Every probe must thus arrive at the listener
inside its class's window, give or take a few microseconds for the
time a frame takes on the wire and for clock error.

The DUT forwards the probes like any other traffic, even though they
are PTP Sync messages: it takes only multicast PTP to its own clock.
"""
import csv
import io
import subprocess
import infamy
import infamy.ptp as ptp
import infamy.qos as qos
from infamy.util import parallel, until

SCHED, VIDEO = (qos.TABLE_34_1[8][prio] for prio in (5, 3))
SCHED_NS, VIDEO_NS = 200_000, 600_000        # gate intervals
CYCLE = 1_000_000
STREAMS = [("EF", 46), ("AF31", 26), ("CS1", 8)]   # (name, DSCP), one per window
GAP = "1.003ms"                              # walks a probe through the cycle in 333 steps
COUNT = 999
SLACK = 5_000                                # ns past a window's end a probe may arrive
TALKER, LISTENER = "192.168.20.1", "192.168.20.2"
SET_GATES = "ieee802-dot1q-sched:set-gate-states"


def gptp(ports, client=False):
    return {"ieee1588-ptp-tt": {"ptp": {"instances": {"instance": [{
        "instance-index": 0,
        "default-ds": {
            "instance-type": "oc" if client else "bc",
            "domain-number": 0,
            "priority1": 128 if client else 1,
            "priority2": 128,
            "infix-ptp:profile": "ieee802-dot1as",
        } | ({"time-receiver-only": True} if client else {}),
        "ports": {"port": [{"port-index": i, "underlying-interface": port}
                           for i, port in enumerate(ports, start=1)]},
    }]}}}}


def station(port, addr):
    return {"ietf-interfaces": {"interfaces": {"interface": [{
        "name": port,
        "enabled": True,
        "ipv4": {"address": [{"ip": addr, "prefix-length": 24}]},
    }]}}} | gptp([port], client=True)


def windows(speed):
    """(gate mask, interval) of each entry, the guard band last"""
    guard = qos.guard_band(speed)
    return [(1 << SCHED, SCHED_NS),
            (1 << VIDEO, VIDEO_NS),
            (0xff & ~(1 << SCHED | 1 << VIDEO), CYCLE - SCHED_NS - VIDEO_NS - guard),
            (0, guard)]


def bridge(uplink, dnlink):
    return {"ietf-interfaces": {"interfaces": {"interface": [
        {"name": "br0", "type": "infix-if-type:bridge", "enabled": True},
        {
            "name": uplink,
            "enabled": True,
            "infix-interfaces:bridge-port": {"bridge": "br0"},
            "infix-interfaces:qos": {"ingress": {"trust": "dscp"}}
        },
        {"name": dnlink, "enabled": True, "infix-interfaces:bridge-port": {"bridge": "br0"}},
    ]}}} | gptp([uplink, dnlink])


def schedule(dnlink, wins):
    return {"ietf-interfaces": {"interfaces": {"interface": [{
        "name": dnlink,
        "infix-interfaces:qos": {"egress": {"traffic-class-table": {"preset": "ieee-sr"}}},
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


def window(wins, i):
    """Start and length of the i:th window"""
    return sum(interval for _, interval in wins[:i]), wins[i][1]


def ptping_cmd(wins):
    streams = " ".join(f"-n {name} -d {dscp} --window {'+'.join(map(str, window(wins, i)))} {LISTENER}"
                       for i, (name, dscp) in enumerate(STREAMS))
    return f"sudo ptping record --cycle {CYCLE} --observe -r {GAP} -c {COUNT} {streams}"


def held_to_window(rows, wins, i):
    """Every probe arrived within its window, at most SLACK after it closed"""
    name = STREAMS[i][0]
    start, length = window(wins, i)
    got = [r for r in rows if r["stream"] == name and r["rx_ns"]]
    assert len(got) >= 0.99 * COUNT, f"{name}: {len(got)} of {COUNT} probes arrived"

    for tag in ("inside", "outside"):
        lat = [int(r["latency_ns"]) for r in got if r["tag"] == tag]
        if lat:
            print(f"{name}: {len(lat)} probes left {tag} the window, "
                  f"latency {min(lat) / 1000:.1f} to {max(lat) / 1000:.1f} µs")

    late = [r for r in got
            if (int(r["rx_phase_ns"]) - start) % CYCLE >= length + SLACK]
    assert not late, \
        f"{name}: {len(late)} probes arrived outside {start // 1000}+{length // 1000} µs, " \
        f"e.g. at {int(late[0]['rx_phase_ns']) / 1000:.1f} µs into the cycle"


with infamy.Test() as test:
    with test.step("Set up topology and attach to the DUT, talker and listener"):
        env = infamy.Env()
        dut, talker, listener, dutssh, talkssh, listssh = parallel(
            lambda: env.attach("dut", "mgmt"),
            lambda: env.attach("talker", "mgmt"),
            lambda: env.attach("listener", "mgmt"),
            lambda: env.attach("dut", "mgmt", "ssh"),
            lambda: env.attach("talker", "mgmt", "ssh"),
            lambda: env.attach("listener", "mgmt", "ssh"))
        uplink, dnlink = dut["uplink"], dut["dnlink"]
        port = infamy.capability.Port(dut, dnlink, dutssh)
        infamy.capability.Port(dut, uplink, dutssh).require_offload(test, "classification")
        if not dut.has_model("ieee802-dot1dc-sched-if"):
            test.skip("the product has no ieee802-dot1dc-sched-if model")
        port.require_classes(test, 8)
        port.require_offload(test, "time-aware-shaper")
        if port.tx_queues < port.traffic_classes:
            test.skip(f"{dnlink} has {port.tx_queues} transmit queues, a schedule needs one per class")
        for name, ssh in (("talker", talkssh), ("listener", listssh)):
            if ssh.runsh("command -v ptping").returncode:
                test.skip(f"the {name} has no ptping")

    with test.step("Bridge the DUT's ports, classify by DSCP, start gPTP"):
        dut.put_config_dicts(bridge(uplink, dnlink))

    with test.step("Set up the talker and listener as gPTP time receivers"):
        parallel(lambda: talker.put_config_dicts(station(talker["data"], TALKER)),
                 lambda: listener.put_config_dicts(station(listener["data"], LISTENER)))

    with test.step("Verify the DUT is grandmaster and the talker and listener follow it"):
        until(lambda: ptp.is_own_gm(dut), attempts=60)
        parallel(*[lambda n=n, t=t: until(lambda: ptp.is_time_receiver(t) and
                                          ptp.has_converged(t, ptp.default_threshold(env, n)),
                                          attempts=120)
                   for n, t in (("talker", talker), ("listener", listener))])

    with test.step("Run the gate control list on the DUT's port toward the listener"):
        wins = windows(int(dut.get_iface(dnlink)["speed"]))
        dut.put_config_dicts(schedule(dnlink, wins))
        until(lambda: "taprio" in {q.get("kind") for q in qos.qdiscs(dutssh, dnlink)})
        qos.show_offload(port, "time-aware-shaper")

    try:
        with test.step("Start a ptping sink on the listener"):
            listssh.runsh(f"sudo pkill ptping; sudo sh -c 'ptping sink {LISTENER} >/dev/null 2>&1 &'")
            out = talkssh.runsh(f"sudo ptping probe {LISTENER}")
            print(out.stdout)
            assert out.returncode == 0, "the listener's sink did not answer a probe"

        with test.step("Send EF, AF31 and CS1 probes through the DUT at every point of the cycle"):
            out = talkssh.run(ptping_cmd(wins), stdout=subprocess.PIPE, text=True)
            assert out.returncode == 0, "ptping failed"
            rows = [r for r in csv.DictReader(io.StringIO(out.stdout)) if r["flow"] == "fwd"]
    finally:
        listssh.runsh("sudo pkill ptping")

    with test.step("Verify EF probes arrived between 0 and 200 µs into the cycle"):
        held_to_window(rows, wins, 0)

    with test.step("Verify AF31 probes arrived between 200 and 800 µs into the cycle"):
        held_to_window(rows, wins, 1)

    with test.step("Verify CS1 probes arrived between 800 µs and the guard band"):
        held_to_window(rows, wins, 2)

    test.succeed()
