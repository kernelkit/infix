#!/usr/bin/env python3
"""
TSN Guard Band

A scheduled port gives time-critical traffic a window of its own in
every cycle.  That only helps if the window starts on time: a large
frame of ordinary traffic that starts just before the window is still
on the wire when it opens.  The guard band prevents this.  The port
holds back a frame that cannot finish before its gate closes, as IEEE
802.1Q-2022 8.6.8.4 specifies.  This test checks that it does, with
the port fully loaded.

.The end of a cycle, with and without the guard band
image::guard-band.svg[align=center, scaledwidth=90%]

The DUT bridges a talker and a listener and is the IEEE 802.1AS (gPTP)
grandmaster on both ports, so all three clocks share its time.  The
port toward the listener runs a 1 ms gate control list: time-critical
traffic, class 5, for the first 200 µs and every other class for the
remaining 800 µs.  The list has no guard band entry of its own, so
holding frames back is left to the port.

The talker floods the port with full-size frames of ordinary traffic,
marked CS1, at 900 Mbps.  That is more than the 800 µs window
carries, so a frame is always waiting when the window closes.  At the
same time it sends time-critical `ptping` probes, marked EF and
timestamped in hardware.  One leaves every 1.003 ms, so the probes
reach the DUT at every point of the cycle in turn.

A probe that reaches the DUT during the CS1 window waits for its gate,
and must arrive at the listener within 5 µs of the gate opening.
Without the guard band a third of them arrive up to 16 µs late.
"""
import csv
import io
import subprocess
import infamy
import infamy.ptp as ptp
import infamy.qos as qos
from infamy.util import parallel, until

SCHED = qos.TABLE_8_5[8][5]                  # class of EF, priority 5
SCHED_NS, CYCLE = 200_000, 1_000_000
WINDOWS = [(1 << SCHED, SCHED_NS),           # (gate mask, interval)
           (0xff & ~(1 << SCHED), CYCLE - SCHED_NS)]
GAP = "1.003ms"                              # walks a probe through the cycle in 333 steps
COUNT = 999
SLACK = 5_000                                # ns after the gate opens a waiting probe may arrive
MARGIN = 50_000                              # ns from the window a waiting probe was sent
FLOOD_MIN = 400_000_000                      # bps of CS1 the DUT must be sending
TALKER, LISTENER = "192.168.20.1", "192.168.20.2"
SET_GATES = "ieee802-dot1q-sched:set-gate-states"
PKTGEN = "/proc/net/pktgen"
FLOOD_MAC = "02:00:00:00:00:01"              # unknown to all: the DUT floods it, the listener drops it


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


def flood(port):
    """Shell command that starts 900 Mbps of full-size CS1 frames through the DUT"""
    params = ["count 0", "pkt_size 1514", "rate 900M", f"dst {LISTENER}", f"dst_mac {FLOOD_MAC}",
              "tos 20", "udp_dst_min 9", "udp_dst_max 9"]
    setup = "; ".join(f"echo {p} > {port}" for p in params)
    return (f"sudo sh -c 'cd {PKTGEN} && echo rem_device_all > kpktgend_0 && "
            f"echo add_device {port} > kpktgend_0 && {setup} && "
            "(echo start > pgctrl) </dev/null >/dev/null 2>&1 &'")


def since_open(phase):
    """Nanoseconds from the gate opening, negative just before it"""
    return (phase + CYCLE // 2) % CYCLE - CYCLE // 2


def schedule(dnlink):
    return {"ietf-interfaces": {"interfaces": {"interface": [{
        "name": dnlink,
        "ieee802-dot1dc-sched-if:gate-parameter-table": {
            "gate-enabled": True,
            "admin-cycle-time": {"numerator": 1, "denominator": 1_000_000_000 // CYCLE},
            "admin-base-time": {"seconds": 0, "nanoseconds": 0},
            "admin-control-list": {"gate-control-entry": [
                {"index": i, "operation-name": SET_GATES,
                 "gate-states-value": mask, "time-interval-value": interval}
                for i, (mask, interval) in enumerate(WINDOWS)]},
        },
    }]}}}


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
        if talkssh.runsh(f"sudo modprobe pktgen; test -e {PKTGEN}/pgctrl").returncode:
            test.skip("the talker has no pktgen")

    with test.step("Bridge the DUT's ports, classify by DSCP, start gPTP"):
        dut.put_config_dicts(bridge(uplink, dnlink))

    with test.step("Set up the talker and listener as gPTP time receivers"):
        parallel(lambda: talker.put_config_dicts(station(talker["data"], TALKER)),
                 lambda: listener.put_config_dicts(station(listener["data"], LISTENER)))

    with test.step("Verify the DUT is grandmaster and the talker and listener follow it"):
        until(lambda: ptp.is_own_gm(dut))
        parallel(*[lambda n=n, t=t: until(lambda: ptp.is_time_receiver(t) and
                                          ptp.has_converged(t, ptp.default_threshold(env, n)),
                                          attempts=120)
                   for n, t in (("talker", talker), ("listener", listener))])

    with test.step("Run the gate control list on the DUT's port toward the listener"):
        dut.put_config_dicts(schedule(dnlink))
        until(lambda: "taprio" in {q.get("kind") for q in qos.qdiscs(dutssh, dnlink)})
        qos.show_offload(port, "time-aware-shaper")

    try:
        with test.step("Start a ptping sink on the listener"):
            listssh.runsh(f"sudo pkill ptping; sudo sh -c 'ptping sink {LISTENER} >/dev/null 2>&1 &'")
            out = talkssh.runsh(f"sudo ptping probe {LISTENER}")
            print(out.stdout)
            assert out.returncode == 0, "the listener's sink did not answer a probe"

        with test.step("Flood CS1 at 900 Mbps through the DUT"):
            out = talkssh.runsh(flood(talker["data"]))
            assert out.returncode == 0, f"pktgen refused the flood: {out.stdout}"
            until(lambda: qos.tx_rate(dutssh, dnlink) > FLOOD_MIN, attempts=10)

        with test.step("Send EF probes through the DUT at every point of the cycle"):
            out = talkssh.run(f"sudo ptping record --cycle {CYCLE} --observe -r {GAP} -c {COUNT} "
                              f"-n EF -d 46 --window 0+{SCHED_NS} {LISTENER}",
                              stdout=subprocess.PIPE, text=True)
            assert out.returncode == 0, "ptping failed"

        with test.step("Verify the CS1 flood ran until the last probe"):
            rate = qos.tx_rate(dutssh, dnlink)
            print(f"{dnlink} sends {rate / 1e6:.0f} Mbps")
            assert rate > FLOOD_MIN, f"the flood fell to {rate / 1e6:.0f} Mbps during the probes"
    finally:
        talkssh.runsh(f"sudo sh -c 'echo stop > {PKTGEN}/pgctrl; "
                      f"echo rem_device_all > {PKTGEN}/kpktgend_0'")
        listssh.runsh("sudo pkill ptping")

    with test.step("Verify EF probes that waited for their gate left as it opened"):
        rows = [r for r in csv.DictReader(io.StringIO(out.stdout))
                if r["flow"] == "fwd" and r["rx_ns"]]
        assert len(rows) >= 0.99 * COUNT, f"{len(rows)} of {COUNT} probes arrived"
        waited = [r for r in rows
                  if SCHED_NS + MARGIN <= int(r["tx_phase_ns"]) <= CYCLE - MARGIN]
        assert waited, "no probe waited for its gate"
        # Probes waiting at one opening leave back to back: check the first
        first = {}
        for r in waited:
            opening = (int(r["rx_ns"]) + CYCLE // 2) // CYCLE
            first[opening] = min(first.get(opening, CYCLE), since_open(int(r["rx_phase_ns"])))
        late = list(first.values())
        print(f"{len(waited)} probes waited at {len(late)} gate openings, the first out "
              f"arrived {min(late) / 1000:.1f} to {max(late) / 1000:.1f} µs after the gate opened")
        assert max(late) < SLACK, \
            f"a probe that waited arrived {max(late) / 1000:.1f} µs after its gate opened"

    test.succeed()
