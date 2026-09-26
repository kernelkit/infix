from .common import LOG
from .host import HOST


def _modem_to_hw_state(modem):
    info = modem.get("info", {})
    status = modem.get("status", {})
    cellular = status.get("cellular", {})

    state = {}
    if info.get("manufacturer"):
        state["manufacturer"] = info["manufacturer"]
    if info.get("model"):
        state["model"] = info["model"]
    if info.get("firmware-version"):
        state["firmware-version"] = info["firmware-version"]
    if info.get("serial-number"):
        state["serial-number"] = info["serial-number"]
    if info.get("imsi"):
        state["imsi"] = info["imsi"]
    if info.get("iccid"):
        state["iccid"] = info["iccid"]
    if status.get("state"):
        state["state"] = status["state"]
    if "signal-quality" in status:
        state["signal-quality"] = status["signal-quality"]
    for sig in ("signal-rssi", "signal-rsrp", "signal-rsrq", "signal-sinr"):
        if sig in status:
            state[sig] = status[sig]

    if cellular:
        cell = {}
        if cellular.get("registration-state"):
            cell["registration-state"] = cellular["registration-state"]
        if cellular.get("operator-name"):
            cell["operator-name"] = cellular["operator-name"]
        if cellular.get("operator-id"):
            cell["operator-id"] = cellular["operator-id"]
        if cellular.get("network-type"):
            cell["network-type"] = cellular["network-type"]
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
        hw_state = _modem_to_hw_state(modem)
        if hw_state:
            hw_components.append({
                "name": name,
                "class": "infix-hardware:modem",
                "infix-hardware:modem-state": hw_state
            })

        sim_raw = modem.get("sim-state")
        if sim_raw:
            sim_name = sim_raw.get("name", "sim%d" % idx)
            sim_hw_state = _sim_to_hw_state(sim_raw)
            if sim_hw_state:
                hw_components.append({
                    "name": sim_name,
                    "class": "infix-hardware:sim",
                    "infix-hardware:sim-state": sim_hw_state
                })

    if not hw_components:
        return {}

    return {
        "ietf-hardware:hardware": {
            "component": hw_components
        }
    }
