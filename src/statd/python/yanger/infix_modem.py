from .common import LOG
from .host import HOST


RUNDIR = "/run/modemd"


# YANG modem-state.state values that map to ietf-hardware oper-state
# 'enabled' (resource is partially or fully operable).  Everything in
# the "failed" / "disabled" buckets maps to oper-state 'disabled';
# everything else is reported as 'unknown'.
_MODEM_OPER_ENABLED = {
    "enabling", "enabled",
    "searching", "registered",
    "connecting", "connected",
    "disconnecting",
}
_MODEM_OPER_DISABLED = {
    "failed", "disabled", "disabling", "locked",
}

# state-failed-reason values that warrant an alarm-state 'major' bit
# (i.e. the operator can resolve them by intervention — insert SIM,
# enter PIN, …).
_SIM_CLASS_FAILED_REASONS = {
    "sim-missing", "sim-error", "sim-wrong", "sim-busy",
    "unlock-required",
}

# A 'warning' alarm-state bit is raised when registered but signal
# quality drops below this percentage.  Recoverable, not load-bearing.
_WEAK_SIGNAL_THRESHOLD = 20


def _location_state(idx):
    data = HOST.read_json("%s/modem%d/location/data.json" % (RUNDIR, idx),
                          default={})
    if not data:
        return None

    state = {}
    for key in ("source", "latitude", "longitude", "altitude",
                "cell-id", "lac", "tac", "mcc", "mnc", "last-change"):
        if key in data and data[key] is not None:
            state[key] = data[key]
    return state or None


def _last_change(idx):
    """modem-state/last-change — freshness, written by modemd each poll."""
    data = HOST.read_json("%s/modem%d/state.json" % (RUNDIR, idx), default={})
    return data.get("last-change") if data else None


def _state_last_changed(idx):
    """component/state/state-last-changed — RFC 4268, transition-driven.

    Written by modemd when admin/oper/alarm state actually changes.
    """
    data = HOST.read_json("%s/modem%d/state.json" % (RUNDIR, idx), default={})
    return data.get("state-last-changed") if data else None


def _modem_oper_state(modem_state):
    if not modem_state:
        return "unknown"
    if modem_state in _MODEM_OPER_ENABLED:
        return "enabled"
    if modem_state in _MODEM_OPER_DISABLED:
        return "disabled"
    return "unknown"


def _modem_alarm_state(modem_state, failed_reason, signal_quality):
    bits = []
    if modem_state == "failed" and failed_reason in _SIM_CLASS_FAILED_REASONS:
        bits.append("major")
    elif modem_state == "failed":
        bits.append("minor")
    elif modem_state in ("registered", "connected") \
            and isinstance(signal_quality, int) \
            and signal_quality < _WEAK_SIGNAL_THRESHOLD:
        bits.append("warning")
    return " ".join(bits) if bits else None


def _sim_oper_state(sim_state):
    if sim_state == "not-inserted":
        return "disabled"
    if sim_state in ("unlocked", "pin-required", "puk-required",
                     "permanently-blocked"):
        return "enabled"
    return "unknown"


def _modem_to_hw_state(modem):
    info = modem.get("info", {})
    status = modem.get("status", {})
    cellular = status.get("cellular", {})

    state = {}

    # Hardware information
    for src_key in ("manufacturer", "model", "hardware-revision",
                    "firmware-version", "serial-number", "imsi", "iccid",
                    "selected-carrier"):
        if info.get(src_key):
            state[src_key] = info[src_key]
        elif status.get(src_key):
            state[src_key] = status[src_key]

    for leaflist_key in ("phone-number", "supported-carrier"):
        values = info.get(leaflist_key) or []
        values = [v for v in values if v]
        if values:
            state[leaflist_key] = values

    # Status
    if status.get("state"):
        state["state"] = status["state"]
    if status.get("state-failed-reason"):
        state["state-failed-reason"] = status["state-failed-reason"]
    if status.get("power-state"):
        state["power-state"] = status["power-state"]
    locks = status.get("enabled-locks") or []
    if locks:
        state["enabled-locks"] = locks

    if "signal-quality" in status:
        state["signal-quality"] = status["signal-quality"]
    if "signal-quality-recent" in status:
        state["signal-quality-recent"] = status["signal-quality-recent"]
    for sig in ("signal-rssi", "signal-rsrp", "signal-rsrq", "signal-sinr"):
        if sig in status:
            state[sig] = status[sig]

    # Cellular
    if cellular:
        cell = {}
        for key in ("registration-state", "service-state",
                    "operator-name", "operator-id", "network-type"):
            v = cellular.get(key)
            if v:
                cell[key] = v
        if cell:
            state["cellular"] = cell

    return state


def _sim_to_hw_state(sim_raw):
    state = {}
    slot = sim_raw.get("slot")
    if isinstance(slot, int):
        state["slot"] = slot
    if sim_raw.get("state"):
        state["state"] = sim_raw["state"]
    if sim_raw.get("operator-name"):
        state["operator-name"] = sim_raw["operator-name"]
    return state


def operational():
    modems = HOST.run_json(['/usr/libexec/modemd/modem-info'], [])

    hw_components = []
    for modem in modems:
        idx = modem.get("index", 0)
        name = "modem%d" % idx
        status = modem.get("status", {})
        modem_state = status.get("state")
        failed_reason = status.get("state-failed-reason")
        signal_quality = status.get("signal-quality")

        hw_state = _modem_to_hw_state(modem)

        location_state = _location_state(idx)
        if location_state:
            hw_state["location-state"] = location_state

        last_change = _last_change(idx)
        if last_change:
            hw_state["last-change"] = last_change

        component = {
            "name": name,
            "class": "infix-hardware:modem",
        }

        # Standard ietf-hardware state container — drives 'show hardware'
        # and gives generic NETCONF clients a meaningful view of the
        # modem without needing modem-specific YANG awareness.
        std_state = {
            "oper-state": _modem_oper_state(modem_state),
        }
        alarm = _modem_alarm_state(modem_state, failed_reason, signal_quality)
        if alarm:
            std_state["alarm-state"] = alarm
        slc = _state_last_changed(idx)
        if slc:
            std_state["state-last-changed"] = slc
        component["state"] = std_state

        if hw_state:
            component["infix-hardware:modem-state"] = hw_state
        hw_components.append(component)

        sim_raw = modem.get("sim-state")
        if sim_raw:
            sim_name = sim_raw.get("name", "sim%d" % idx)
            sim_hw_state = _sim_to_hw_state(sim_raw)
            sim_component = {
                "name": sim_name,
                "class": "infix-hardware:sim",
                "state": {
                    "oper-state": _sim_oper_state(sim_raw.get("state")),
                },
            }
            if slc:
                sim_component["state"]["state-last-changed"] = slc
            if sim_hw_state:
                sim_component["infix-hardware:sim-state"] = sim_hw_state
            hw_components.append(sim_component)

    if not hw_components:
        return {}

    return {
        "ietf-hardware:hardware": {
            "component": hw_components
        }
    }
