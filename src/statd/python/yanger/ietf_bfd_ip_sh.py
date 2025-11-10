from .common import insert
from .host import HOST


def frr_to_ietf_state(state):
    """Convert FRR BFD state to IETF BFD state"""
    state_map = {
        "up": "up",
        "down": "down",
        "init": "init",
        "adminDown": "adminDown"
    }
    return state_map.get(state, "down")


def add_sessions(control_protocols):
    """Fetch BFD session data from FRR"""
    cmd = ['vtysh', '-c', 'show bfd peers json']
    data = HOST.run_json(cmd, default=[])
    if not data:
        return  # No BFD sessions available

    control_protocol = {}
    control_protocol["type"] = "infix-routing:bfdv1"
    control_protocol["name"] = "bfd"
    control_protocol["ietf-bfd:bfd"] = {}
    control_protocol["ietf-bfd:bfd"]["ietf-bfd-ip-sh:ip-sh"] = {}
    control_protocol["ietf-bfd:bfd"]["ietf-bfd-ip-sh:ip-sh"]["sessions"] = {}

    sessions = []

    # FRR returns a list of BFD peers
    for peer in data:
        # Only process single-hop sessions (multihop == false)
        if peer.get("multihop", False):
            continue

        session = {}

        # Key fields: interface and dest-addr
        session["interface"] = peer.get("interface", "unknown")
        session["dest-addr"] = peer.get("peer", "0.0.0.0")

        # Operational state fields (config false)
        # Local and remote discriminators
        if peer.get("id") is not None:
            session["local-discriminator"] = peer["id"]
        if peer.get("remote-id") is not None:
            session["remote-discriminator"] = peer["remote-id"]

        # Session running state
        session["session-running"] = {}

        # Local and remote state
        state = peer.get("status", "down")
        session["session-running"]["local-state"] = frr_to_ietf_state(state)
        # Remote state not directly available in FRR output, infer from status
        session["session-running"]["remote-state"] = frr_to_ietf_state(state)

        # Local diagnostic - not directly available, use "none"
        session["session-running"]["local-diagnostic"] = "none"

        # Detection mode - FRR uses async mode for OSPF-created sessions
        session["session-running"]["detection-mode"] = "async-without-echo"

        # Timing intervals (convert milliseconds to microseconds for YANG)
        if peer.get("receive-interval") is not None:
            session["session-running"]["negotiated-rx-interval"] = peer["receive-interval"] * 1000
        if peer.get("transmit-interval") is not None:
            session["session-running"]["negotiated-tx-interval"] = peer["transmit-interval"] * 1000

        # Detection time (in microseconds)
        if peer.get("detect-multiplier") is not None and peer.get("receive-interval") is not None:
            detection_time_ms = peer["detect-multiplier"] * peer["receive-interval"]
            session["session-running"]["detection-time"] = detection_time_ms * 1000

        # Path type
        session["path-type"] = "ietf-bfd-types:path-ip-sh"
        session["ip-encapsulation"] = True

        sessions.append(session)

    if sessions:
        control_protocol["ietf-bfd:bfd"]["ietf-bfd-ip-sh:ip-sh"]["sessions"]["session"] = sessions
        insert(control_protocols, "control-plane-protocol", [control_protocol])


def operational():
    out = {
        "ietf-routing:routing": {
            "control-plane-protocols": {}
        }
    }

    add_sessions(out['ietf-routing:routing']['control-plane-protocols'])
    return out
