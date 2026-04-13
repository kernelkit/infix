"""PTP (IEEE 1588) test helpers

Query PTP operational data from the ieee1588-ptp-tt YANG model.
All functions are None-safe and intended for use with until():

    until(lambda: ptp.is_time_receiver(target), attempts=60)
"""


def _get_instance(target, idx=0):
    data = target.get_data("/ieee1588-ptp-tt:ptp") or {}
    instances = (data.get("ptp", {})
                     .get("instances", {})
                     .get("instance", []))
    for inst in instances:
        if inst.get("instance-index") == idx:
            return inst
    return None


def port_state(target, port_idx=1, inst_idx=0):
    """Return port-state string for given port, or None."""
    inst = _get_instance(target, inst_idx)
    if not inst:
        return None
    for port in inst.get("ports", {}).get("port", []):
        if port.get("port-index") == port_idx:
            return port.get("port-ds", {}).get("port-state")
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
    raw = inst.get("current-ds", {}).get("offset-from-time-transmitter")
    try:
        return int(raw) // 65536
    except (TypeError, ValueError):
        return None


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
    """True when |offset-from-time-transmitter| < threshold_ns."""
    off = offset_ns(target, inst_idx)
    if off is None:
        return False
    return abs(off) < threshold_ns


def port_state_dbg(target, port_idx=1, inst_idx=0):
    """Return a diagnostic string with instance/port state, or an error hint.

    Useful in until() lambdas and test step output to show what is actually
    being observed when a state check does not converge::

        until(lambda: is_time_transmitter(gm) or not print(port_state_dbg(gm)),
              attempts=60)
    """
    data = target.get_data("/ieee1588-ptp-tt:ptp") or {}
    if not data:
        return f"{target.name}: no PTP operational data (ptp4l not running?)"

    instances = (data.get("ptp", {})
                     .get("instances", {})
                     .get("instance", []))
    if not instances:
        return f"{target.name}: PTP data present but no instances"

    parts = []
    for inst in instances:
        idx = inst.get("instance-index", "?")
        for port in inst.get("ports", {}).get("port", []):
            pidx  = port.get("port-index", "?")
            state = port.get("port-ds", {}).get("port-state", "?")
            parts.append(f"inst={idx} port={pidx} state={state}")

    return f"{target.name}: " + (", ".join(parts) if parts else "no ports")


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
