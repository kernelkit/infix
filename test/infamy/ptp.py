"""PTP (IEEE 1588) test helpers

Query PTP operational data from the ieee1588-ptp-tt YANG model.
All functions are None-safe and intended for use with until():

    until(lambda: ptp.is_time_receiver(target), attempts=60)

Port state changes are logged as they are seen, and if the test fails the
PTP state of every DUT queried here is dumped to the test log.
"""

import subprocess

from infamy import ssh, tap

_targets = {}
_states = {}


def _instances(target):
    _targets[target.name] = target
    data = target.get_data("/ieee1588-ptp-tt:ptp") or {}
    return (data.get("ptp", {})
                .get("instances", {})
                .get("instance", []))


def _get_instance(target, idx=0):
    return next((inst for inst in _instances(target)
                 if inst.get("instance-index") == idx), None)


def _scaled_ns(value):
    """Scaled nanoseconds (int64 x 2^16, as a string) to nanoseconds."""
    try:
        return int(value) // 65536
    except (TypeError, ValueError):
        return None


def port_state(target, port_idx=1, inst_idx=0):
    """Return port-state string for given port, or None."""
    inst = _get_instance(target, inst_idx)
    if not inst:
        return None
    for port in inst.get("ports", {}).get("port", []):
        if port.get("port-index") == port_idx:
            state = port.get("port-ds", {}).get("port-state")
            key = (target.name, inst_idx, port_idx)
            if _states.get(key) != state:
                print(f"{target.name}: port {port_idx} "
                      f"({port.get('underlying-interface', '?')}) {state}")
                _states[key] = state
            return state
    return None


def is_time_receiver(target, port_idx=1, inst_idx=0):
    """True when port is in time-receiver state."""
    return port_state(target, port_idx, inst_idx) == "time-receiver"


def is_time_transmitter(target, port_idx=1, inst_idx=0):
    """True when port is in time-transmitter state."""
    return port_state(target, port_idx, inst_idx) == "time-transmitter"


def offset_ns(target, inst_idx=0):
    """Return offset-from-time-transmitter in nanoseconds, or None.

    The YANG value is scaled nanoseconds (int64 × 2^16 stored as string).
    """
    inst = _get_instance(target, inst_idx)
    if not inst:
        return None
    return _scaled_ns(inst.get("current-ds", {})
                         .get("offset-from-time-transmitter"))


def steps_removed(target, inst_idx=0):
    """Return steps-removed count, or None."""
    inst = _get_instance(target, inst_idx)
    return inst.get("current-ds", {}).get("steps-removed") if inst else None


def grandmaster_identity(target, inst_idx=0):
    """Return grandmaster-identity string from parent-ds, or None."""
    inst = _get_instance(target, inst_idx)
    return inst.get("parent-ds", {}).get("grandmaster-identity") if inst else None


def clock_identity(target, inst_idx=0):
    """Return this device's clock-identity string from default-ds, or None."""
    inst = _get_instance(target, inst_idx)
    return inst.get("default-ds", {}).get("clock-identity") if inst else None


def is_own_gm(target, inst_idx=0):
    """True when device is its own grandmaster (acting as GM).

    Compares clock-identity to grandmaster-identity; equal means the
    device won the BTCA election and is distributing its own time.
    """
    cid = clock_identity(target, inst_idx)
    gm = grandmaster_identity(target, inst_idx)
    return cid is not None and cid == gm


def has_converged(target, threshold_ns=100_000, inst_idx=0):
    """True when |offset-from-time-transmitter| < threshold_ns.

    Prints the offset while unconverged, so a failed until() wait
    shows the offset trend in the test log.
    """
    off = offset_ns(target, inst_idx)
    if off is None:
        print(f"{target.name}: no offset-from-time-transmitter")
        return False
    if abs(off) < threshold_ns:
        return True
    print(f"{target.name}: offset {off} ns, threshold {threshold_ns} ns")
    return False


def port_state_dbg(target, port_idx=1, inst_idx=0):
    """Return a diagnostic string with instance/port state, or an error hint.

    Useful in until() lambdas and test step output to show what is actually
    being observed when a state check does not converge::

        until(lambda: is_time_transmitter(gm) or not print(port_state_dbg(gm)),
              attempts=60)
    """
    instances = _instances(target)
    if not instances:
        return f"{target.name}: no PTP instances (ptp4l not running?)"

    parts = []
    for inst in instances:
        idx = inst.get("instance-index", "?")
        for port in inst.get("ports", {}).get("port", []):
            pidx  = port.get("port-index", "?")
            state = port.get("port-ds", {}).get("port-state", "?")
            parts.append(f"inst={idx} port={pidx} state={state}")

    return f"{target.name}: " + (", ".join(parts) if parts else "no ports")


def _ns(value):
    ns = _scaled_ns(value)
    return "?" if ns is None else ns


# Two samples, 2 s apart, of every ptp4l port's message counters and of the
# switch and kernel counters of the PTP ports.  Prints the non-zero ones
# with their change between the samples.
_COUNTERS_SH = """
snap() {
    for c in $(sudo -n sh -c 'ls /etc/linuxptp/ptp4l-*.conf' 2>/dev/null); do
        i=${c##*/}
        sudo -n pmc -u -b 0 -f "$c" "GET PORT_STATS_NP" 2>/dev/null |
            awk -v i="${i%.conf}" '/portIdentity/ {p = $2}
                                  $1 ~ /^[rt]x_/ {print i, p, $1, $2}'
    done
    for n in @IFACES@; do
        ethtool -S "$n" 2>/dev/null |
            awk -v n="$n" 'NR > 1 {sub(":", "", $1); print n, "switch", $1, $2}'
        for s in rx_packets rx_dropped rx_errors tx_packets tx_dropped tx_errors; do
            echo "$n kernel $s $(cat /sys/class/net/$n/statistics/$s 2>/dev/null)"
        done
    done
}
a=$(mktemp); b=$(mktemp)
snap > "$a"; sleep 2; snap > "$b"
awk 'NR == FNR {v[$1 " " $2 " " $3] = $4; next}
     $4 > 0 {printf "%s %s %s=%s(+%d)\\n", $1, $2, $3, $4, $4 - v[$1 " " $2 " " $3]}' "$a" "$b"
rm -f "$a" "$b"
"""


def _counters(target, ifaces):
    """Print ptp4l, switch and kernel counters of target's PTP ports, over SSH."""
    dut = ssh.device_for(target)
    script = _COUNTERS_SH.replace("@IFACES@", " ".join(sorted(ifaces)))
    try:
        res = dut.runsh(script, timeout=30)
    except subprocess.TimeoutExpired:
        print(f"{target.name}:   cannot read counters: timed out")
        return
    if res.returncode:
        print(f"{target.name}:   cannot read counters: {res.stdout.strip()}")
        return
    lines = {}
    for line in res.stdout.splitlines():
        fields = line.split(" ", 2)
        if len(fields) == 3:
            lines.setdefault(" ".join(fields[:2]), []).append(fields[2])
    for key, cnts in lines.items():
        print(f"{target.name}:   {key}: {' '.join(cnts)}")


def dump(target):
    """Print the PTP state of target: clocks, ports, and message counters.

    The gPTP counters tell apart a port that receives nothing, one whose
    peer delay requests go unanswered, and one that ignores what it gets.
    The switch and kernel counters tell whether frames left or reached
    the port at all.
    """
    instances = _instances(target)
    if not instances:
        print(f"{target.name}: no PTP instances (ptp4l not running?)")
        return

    ifaces = set()
    for inst in instances:
        dds = inst.get("default-ds", {})
        cds = inst.get("current-ds", {})
        pds = inst.get("parent-ds", {})
        print(f"{target.name}: instance {inst.get('instance-index')} "
              f"{dds.get('instance-type', '?')} "
              f"clock {dds.get('clock-identity', '?')} "
              f"gm {pds.get('grandmaster-identity', '?')} "
              f"steps-removed {cds.get('steps-removed', '?')} "
              f"offset {_ns(cds.get('offset-from-time-transmitter'))} ns "
              f"mean-delay {_ns(cds.get('mean-delay'))} ns")
        for port in inst.get("ports", {}).get("port", []):
            ds = port.get("port-ds", {})
            if port.get("underlying-interface"):
                ifaces.add(port["underlying-interface"])
            print(f"{target.name}:   port {port.get('port-index')} "
                  f"({port.get('underlying-interface', '?')}) "
                  f"{ds.get('port-state', '?')} "
                  f"link-delay {_ns(ds.get('mean-link-delay'))} ns "
                  f"{ds.get('delay-mechanism', '?')}")
            stats = port.get("ieee802-dot1as-gptp:port-statistics-ds")
            if stats:
                cnt = " ".join(f"{k.removesuffix('-count')}={v}"
                               for k, v in stats.items())
                print(f"{target.name}:     {cnt}")

    print(f"{target.name}: counters, change over 2 s in parentheses:")
    _counters(target, ifaces)


def dump_all():
    """Print the PTP state of every DUT the helpers have queried."""
    if not _targets:
        return
    print("PTP state of the DUTs at failure:")
    for target in list(_targets.values()):
        try:
            dump(target)
        except Exception as e:
            print(f"{target.name}: cannot read PTP state: {e}")


def default_threshold(env, logical_node, hops=1):
    """Return a convergence threshold suited to the node's timestamping capability.

    Queries the physical topology for ptp-hwts on any link connected to the
    physical node matched to logical_node.  Returns 1000 ns (1 µs) per hop for
    hardware-timestamping nodes or 100000 ns (100 µs) for software timestamping.

    Use hops=2 for a receiver behind a Boundary Clock — each BC hop adds
    phc2sys relay jitter on multi-chip hardware.

    Pass --threshold-ns on the command line to override.
    """
    phys = env.ltop.xlate(logical_node)
    g = env.ptop.g
    has_hwts = any(
        "ptp-hwts" in data.get("provides", set())
        for _, _, data in g.edges(phys, data=True)
    )
    return 1_000 * hops if has_hwts else 100_000


tap.on_failure(dump_all)
