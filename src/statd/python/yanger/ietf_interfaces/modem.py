"""
Modem (wwan) interface operational state.

Populates `infix-interfaces:wwan/bearer-state` from modemd's modem-info
JSON output.  Yanger calls wwan(ifname) from link.interface() when an
interface is classified as 'infix-if-type:modem'.

The mapping from wwan interface to bearer:

  - modem-info reports a list of bearers per modem.  Each bearer dict
    has an 'interface' field set to the kernel device name (e.g.
    'wwan0') when the bearer is connected; absent when disconnected.
  - For a connected bearer we match by 'interface' directly.
  - For disconnected bearers we fall back to modem index: bearers on
    'modem{N}' default to 'wwan{N}' when no better hint is available.

That matches modemd's own naming convention for the single-bearer
case, which is the only case the codebase exercises today.  Multi-
bearer setups will need a richer cross-reference (e.g. APN matching
against sysrepo config) — a follow-on once multi-bearer is in
serious use.
"""

from ..host import HOST


# modem-info bearer key -> bearer-state YANG leaf.  Keys absent from
# the source dict are simply not emitted.
_BEARER_KEY_MAP = {
    "path":                       "path",
    "connected":                  "connected",
    "connection-failed-reason":   "connection-failed-reason",
    "interface":                  "interface",
    "ipv4-address":               "ipv4-address",
    "ipv4-prefix":                "ipv4-prefix-length",
    "ipv6-address":               "ipv6-address",
    "ipv6-prefix":                "ipv6-prefix-length",
    "in-bytes":                   "in-bytes",
    "out-bytes":                  "out-bytes",
    "total-in-bytes":             "total-in-bytes",
    "total-out-bytes":            "total-out-bytes",
    "total-duration":             "total-duration",
}


def _bearer_state(bearer):
    state = {}
    for src, dst in _BEARER_KEY_MAP.items():
        if src not in bearer:
            continue
        val = bearer[src]
        if val in (None, "", "--"):
            continue
        if dst in ("ipv4-prefix-length", "ipv6-prefix-length"):
            try:
                val = int(val)
            except (TypeError, ValueError):
                continue
            if val == 0:
                continue
        state[dst] = val
    return state or None


def _find_bearer(modems, ifname):
    """Return the bearer dict matching ifname, or None.

    Prefers an exact 'interface' match (connected case), falls back to
    the bearer list of modem{N} when ifname is wwan{N}.
    """
    for modem in modems:
        bearers = (modem.get("status") or {}).get("bearer") or []
        for b in bearers:
            if b.get("interface") == ifname:
                return b

    # No connected match — fall back to wwan{N} → modem{N}.
    if ifname.startswith("wwan"):
        try:
            idx = int(ifname[4:])
        except ValueError:
            return None
        for modem in modems:
            if modem.get("index") == idx:
                bearers = (modem.get("status") or {}).get("bearer") or []
                if bearers:
                    return bearers[0]
    return None


def wwan(ifname):
    modems = HOST.run_json(['/usr/libexec/modemd/modem-info'], [])
    if not modems:
        return None

    bearer = _find_bearer(modems, ifname)
    if not bearer:
        return None

    state = _bearer_state(bearer)
    if not state:
        return None
    return {"bearer-state": state}
