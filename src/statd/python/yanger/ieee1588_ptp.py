"""Operational data for ieee1588-ptp-tt (and ieee802-dot1as-gptp).

Queries each running ptp4l instance via pmc and maps the output to the
YANG model structure.  One ptp4l process runs per instance-index, with
its config at /etc/linuxptp/ptp4l-<idx>.conf and its UDS socket at
/var/run/ptp4l-<idx>.
"""

import glob
import os
import re

from .common import insert, LOG
from .host import HOST


# ---------------------------------------------------------------------------
# pmc helpers
# ---------------------------------------------------------------------------

def _pmc_get(conf, command):
    """Run 'pmc -b 0 -f <conf> GET <command>' and return parsed key→value dict.

    pmc output looks like:
        \t\t<key>   <value>
    Blank lines and lines not starting with whitespace are ignored.
    Multiple response blocks (one per port for PORT_DATA_SET) each get
    their own dict; returns a list of dicts in that case.
    """
    lines = HOST.run_multiline(
        ["pmc", "-u", "-b", "0", "-f", conf, f"GET {command}"], default=[])

    blocks = []
    current = {}
    for line in lines:
        stripped = line.strip()
        if not stripped or stripped.startswith("sending:") or \
                "RESPONSE MANAGEMENT" in stripped or \
                "SIGNALING" in stripped:
            if current:
                blocks.append(current)
                current = {}
            continue
        m = re.match(r'^\s+(\S+)\s+(.+)$', line)
        if m:
            current[m.group(1)] = m.group(2).strip()

    if current:
        blocks.append(current)

    return blocks


def _pmc_get_one(conf, command):
    """Like _pmc_get but return only the first (or only) block."""
    blocks = _pmc_get(conf, command)
    return blocks[0] if blocks else {}


# ---------------------------------------------------------------------------
# clockIdentity formatting
# ---------------------------------------------------------------------------

def _fmt_clock_identity(raw):
    """Convert pmc clockIdentity 'aabbcc.fffe.ddeeff' to YANG format 'AA-BB-CC-FF-FE-DD-EE-FF'.

    The YANG typedef clock-identity requires the pattern [0-9A-F]{2}(-[0-9A-F]{2}){7}.
    pmc outputs in its own dotted notation e.g. '005182.fffe.112202'.
    """
    raw = raw.replace(".", "").replace("-", "").replace(":", "").upper()
    if len(raw) == 16:
        return "-".join(raw[i:i+2] for i in range(0, 16, 2))
    return raw


def _fmt_port_identity(raw):
    """Convert 'aabbccfffe001122-1' to dict {clock-identity, port-number}."""
    parts = raw.rsplit("-", 1)
    cid = _fmt_clock_identity(parts[0]) if parts else raw
    pnum = int(parts[1]) if len(parts) == 2 else 0
    return {"clock-identity": cid, "port-number": pnum}


# ---------------------------------------------------------------------------
# clockAccuracy identity mapping
# ---------------------------------------------------------------------------

# Map clockClass decimal values to ieee1588-ptp-tt identity names (identityref, not uint8).
_CLOCK_CLASS_MAP = {
    6:   "ieee1588-ptp-tt:cc-primary-sync",
    7:   "ieee1588-ptp-tt:cc-primary-sync-lost",
    13:  "ieee1588-ptp-tt:cc-application-specific-sync",
    14:  "ieee1588-ptp-tt:cc-application-specific-sync-lost",
    52:  "ieee1588-ptp-tt:cc-primary-sync-alternative-a",
    58:  "ieee1588-ptp-tt:cc-application-specific-alternative-a",
    187: "ieee1588-ptp-tt:cc-primary-sync-alternative-b",
    193: "ieee1588-ptp-tt:cc-application-specific-alternative-b",
    248: "ieee1588-ptp-tt:cc-default",
    255: "ieee1588-ptp-tt:cc-time-receiver-only",
}


def _clock_class_identity(raw):
    """Return the YANG identity string for a pmc clockClass decimal value, or None."""
    try:
        return _CLOCK_CLASS_MAP.get(int(raw))
    except (ValueError, TypeError):
        return None


# Map clockAccuracy hex values to ieee1588-ptp-tt identity names (identityref, not uint8).
# Identity names use the 'ca-' prefix as defined in ieee1588-ptp-tt@2023-08-14.yang.
# 0xfe (unknown) has no corresponding identity and is omitted by returning None.
_CLOCK_ACCURACY_MAP = {
    0x17: "ieee1588-ptp-tt:ca-time-accurate-to-1000-fs",
    0x18: "ieee1588-ptp-tt:ca-time-accurate-to-2500-fs",
    0x19: "ieee1588-ptp-tt:ca-time-accurate-to-10-ps",
    0x1a: "ieee1588-ptp-tt:ca-time-accurate-to-25ps",
    0x1b: "ieee1588-ptp-tt:ca-time-accurate-to-100-ps",
    0x1c: "ieee1588-ptp-tt:ca-time-accurate-to-250-ps",
    0x1d: "ieee1588-ptp-tt:ca-time-accurate-to-1000-ps",
    0x1e: "ieee1588-ptp-tt:ca-time-accurate-to-2500-ps",
    0x1f: "ieee1588-ptp-tt:ca-time-accurate-to-10-ns",
    0x20: "ieee1588-ptp-tt:ca-time-accurate-to-25-ns",
    0x21: "ieee1588-ptp-tt:ca-time-accurate-to-100-ns",
    0x22: "ieee1588-ptp-tt:ca-time-accurate-to-250-ns",
    0x23: "ieee1588-ptp-tt:ca-time-accurate-to-1000-ns",
    0x24: "ieee1588-ptp-tt:ca-time-accurate-to-2500-ns",
    0x25: "ieee1588-ptp-tt:ca-time-accurate-to-10-us",
    0x26: "ieee1588-ptp-tt:ca-time-accurate-to-25-us",
    0x27: "ieee1588-ptp-tt:ca-time-accurate-to-100-us",
    0x28: "ieee1588-ptp-tt:ca-time-accurate-to-250-us",
    0x29: "ieee1588-ptp-tt:ca-time-accurate-to-1000-us",
    0x2a: "ieee1588-ptp-tt:ca-time-accurate-to-2500-us",
    0x2b: "ieee1588-ptp-tt:ca-time-accurate-to-10-ms",
    0x2c: "ieee1588-ptp-tt:ca-time-accurate-to-25-ms",
    0x2d: "ieee1588-ptp-tt:ca-time-accurate-to-100-ms",
    0x2e: "ieee1588-ptp-tt:ca-time-accurate-to-250-ms",
    0x2f: "ieee1588-ptp-tt:ca-time-accurate-to-1-s",
    0x30: "ieee1588-ptp-tt:ca-time-accurate-to-10-s",
    0x31: "ieee1588-ptp-tt:ca-time-accurate-to-gt-10-s",
}


def _clock_accuracy_identity(raw):
    """Return the YANG identity string for a pmc clockAccuracy hex value, or None."""
    try:
        return _CLOCK_ACCURACY_MAP.get(int(raw, 16))
    except (ValueError, TypeError):
        return None


# ---------------------------------------------------------------------------
# time-source identity mapping
# ---------------------------------------------------------------------------

_TIME_SOURCE_MAP = {
    "0x10": "ieee1588-ptp-tt:atomic-clock",
    "0x20": "ieee1588-ptp-tt:gnss",
    "0x30": "ieee1588-ptp-tt:terrestrial-radio",
    "0x39": "ieee1588-ptp-tt:serial-time-code",
    "0x40": "ieee1588-ptp-tt:ptp",
    "0x50": "ieee1588-ptp-tt:ntp",
    "0x60": "ieee1588-ptp-tt:hand-set",
    "0x90": "ieee1588-ptp-tt:other",
    "0xa0": "ieee1588-ptp-tt:internal-oscillator",
}


def _time_source_identity(raw):
    return _TIME_SOURCE_MAP.get(raw.lower(),
                                "ieee1588-ptp-tt:internal-oscillator")


# ---------------------------------------------------------------------------
# delay-mechanism and port-state mapping
# ---------------------------------------------------------------------------

_DELAY_MECH_MAP = {
    "E2E":  "e2e",
    "P2P":  "p2p",
    "AUTO": "no-mechanism",
}

_PORT_STATE_MAP = {
    "INITIALIZING":     "initializing",
    "FAULTY":           "faulty",
    "DISABLED":         "disabled",
    "LISTENING":        "listening",
    "PRE_MASTER":       "pre-time-transmitter",
    "MASTER":           "time-transmitter",
    "PASSIVE":          "passive",
    "UNCALIBRATED":     "uncalibrated",
    "SLAVE":            "time-receiver",
    "GRAND_MASTER":     "time-transmitter",
}


# ---------------------------------------------------------------------------
# Per-dataset builders
# ---------------------------------------------------------------------------

def _build_default_ds(d):
    """Map pmc DEFAULT_DATA_SET response to YANG default-ds."""
    ds = {}

    cid = d.get("clockIdentity")
    if cid:
        ds["clock-identity"] = _fmt_clock_identity(cid)

    v = d.get("numberPorts")
    if v:
        ds["number-ports"] = int(v)

    cq = {}
    v = d.get("clockClass")
    if v:
        cc = _clock_class_identity(v)
        if cc:
            cq["clock-class"] = cc
    v = d.get("clockAccuracy")
    if v:
        ca = _clock_accuracy_identity(v)
        if ca:
            cq["clock-accuracy"] = ca
    v = d.get("offsetScaledLogVariance")
    if v:
        cq["offset-scaled-log-variance"] = int(v, 16)
    if cq:
        ds["clock-quality"] = cq

    v = d.get("priority1")
    if v:
        ds["priority1"] = int(v)
    v = d.get("priority2")
    if v:
        ds["priority2"] = int(v)

    v = d.get("domainNumber")
    if v:
        ds["domain-number"] = int(v)

    v = d.get("clientOnly") or d.get("slaveOnly")  # renamed in ptp4l 4.x
    if v is not None:
        ds["time-receiver-only"] = (v == "1")

    # instance-type: derive from ptp4l GM/time-receiver state (read-only, operational)
    # pmc doesn't directly expose clockType in DEFAULT_DATA_SET
    # We'll fill instance-type later from the instance's config if possible

    return ds


def _build_current_ds(d):
    """Map pmc CURRENT_DATA_SET response to YANG current-ds."""
    ds = {}

    v = d.get("stepsRemoved")
    if v:
        ds["steps-removed"] = int(v)

    v = d.get("offsetFromMaster")
    if v:
        # ptp4l reports nanoseconds as float; YANG time-interval is ns * 2^16.
        # RFC 7951: int64 must be JSON-encoded as a string.
        try:
            ds["offset-from-time-transmitter"] = str(int(float(v) * 65536))
        except ValueError:
            pass

    v = d.get("meanPathDelay")
    if v:
        try:
            ds["mean-delay"] = str(int(float(v) * 65536))
        except ValueError:
            pass

    return ds


def _build_parent_ds(d):
    """Map pmc PARENT_DATA_SET response to YANG parent-ds."""
    ds = {}

    v = d.get("parentPortIdentity")
    if v:
        ds["parent-port-identity"] = _fmt_port_identity(v)

    v = d.get("parentStats")
    if v:
        ds["parent-stats"] = (v == "1")

    v = d.get("observedParentOffsetScaledLogVariance")
    if v:
        try:
            ds["observed-parent-offset-scaled-log-variance"] = int(v, 16)
        except ValueError:
            pass

    v = d.get("observedParentClockPhaseChangeRate")
    if v:
        try:
            ds["observed-parent-clock-phase-change-rate"] = int(v)
        except ValueError:
            pass

    v = d.get("grandmasterIdentity")
    if v:
        ds["grandmaster-identity"] = _fmt_clock_identity(v)

    gcq = {}
    v = d.get("gm.ClockClass")
    if v:
        cc = _clock_class_identity(v)
        if cc:
            gcq["clock-class"] = cc
    v = d.get("gm.ClockAccuracy")
    if v:
        ca = _clock_accuracy_identity(v)
        if ca:
            gcq["clock-accuracy"] = ca
    v = d.get("gm.OffsetScaledLogVariance")
    if v:
        try:
            gcq["offset-scaled-log-variance"] = int(v, 16)
        except ValueError:
            pass
    if gcq:
        ds["grandmaster-clock-quality"] = gcq

    v = d.get("grandmasterPriority1")
    if v:
        ds["grandmaster-priority1"] = int(v)
    v = d.get("grandmasterPriority2")
    if v:
        ds["grandmaster-priority2"] = int(v)

    return ds


def _build_time_properties_ds(d):
    """Map pmc TIME_PROPERTIES_DATA_SET response to YANG time-properties-ds."""
    ds = {}

    # current-utc-offset has a when condition requiring current-utc-offset-valid='true'
    if d.get("currentUtcOffsetValid") in ("1", "true"):
        v = d.get("currentUtcOffset")
        if v:
            ds["current-utc-offset"] = int(v)

    v = d.get("leap61")
    if v is not None:
        ds["leap61"] = (v == "1")
    v = d.get("leap59")
    if v is not None:
        ds["leap59"] = (v == "1")
    v = d.get("currentUtcOffsetValid")
    if v is not None:
        ds["current-utc-offset-valid"] = (v == "1")
    v = d.get("ptpTimescale")
    if v is not None:
        ds["ptp-timescale"] = (v == "1")
    v = d.get("timeTraceable")
    if v is not None:
        ds["time-traceable"] = (v == "1")
    v = d.get("frequencyTraceable")
    if v is not None:
        ds["frequency-traceable"] = (v == "1")

    v = d.get("timeSource")
    if v:
        ds["time-source"] = _time_source_identity(v)

    return ds


def _build_port_ds(d):
    """Map pmc PORT_DATA_SET response to YANG port-ds."""
    ds = {}

    v = d.get("portIdentity")
    if v:
        ds["port-identity"] = _fmt_port_identity(v)

    v = d.get("portState")
    if v:
        ds["port-state"] = _PORT_STATE_MAP.get(v, "disabled")

    v = d.get("logMinDelayReqInterval")
    if v:
        try:
            ds["log-min-delay-req-interval"] = int(v)
        except ValueError:
            pass

    v = d.get("peerMeanPathDelay")
    if v:
        try:
            # RFC 7951: int64 must be JSON-encoded as a string.
            ds["mean-link-delay"] = str(int(float(v) * 65536))
        except ValueError:
            pass

    v = d.get("logAnnounceInterval")
    if v:
        try:
            ds["log-announce-interval"] = int(v)
        except ValueError:
            pass

    v = d.get("announceReceiptTimeout")
    if v:
        try:
            ds["announce-receipt-timeout"] = int(v)
        except ValueError:
            pass

    v = d.get("logSyncInterval")
    if v:
        try:
            ds["log-sync-interval"] = int(v)
        except ValueError:
            pass

    v = d.get("delayMechanism")
    if v:
        ds["delay-mechanism"] = _DELAY_MECH_MAP.get(v, "e2e")

    v = d.get("logMinPdelayReqInterval")
    if v:
        try:
            ds["log-min-pdelay-req-interval"] = int(v)
        except ValueError:
            pass

    v = d.get("versionNumber")
    if v:
        try:
            ds["version-number"] = int(v)
        except ValueError:
            pass

    v = d.get("portEnable")
    if v is not None:
        ds["port-enable"] = (v == "1")

    return ds


def _build_port_stats(d):
    """Map pmc PORT_STATS_NP response to ieee802-dot1as-gptp port-statistics-ds."""
    stats = {}
    mapping = {
        "rx_Sync":                  "rx-sync-count",
        "rx_Follow_Up":             "rx-follow-up-count",
        "rx_Pdelay_Req":            "rx-pdelay-req-count",
        "rx_Pdelay_Resp":           "rx-pdelay-resp-count",
        "rx_Pdelay_Resp_Follow_Up": "rx-pdelay-resp-follow-up-count",
        "rx_Announce":              "rx-announce-count",
        "tx_Sync":                  "tx-sync-count",
        "tx_Follow_Up":             "tx-follow-up-count",
        "tx_Pdelay_Req":            "tx-pdelay-req-count",
        "tx_Pdelay_Resp":           "tx-pdelay-resp-count",
        "tx_Pdelay_Resp_Follow_Up": "tx-pdelay-resp-follow-up-count",
        "tx_Announce":              "tx-announce-count",
    }
    for pmc_key, yang_key in mapping.items():
        v = d.get(pmc_key)
        if v is not None:
            try:
                stats[yang_key] = int(v)
            except ValueError:
                pass
    return stats


# ---------------------------------------------------------------------------
# Per-instance builder
# ---------------------------------------------------------------------------

def _port_interfaces(conf_path):
    """Return ordered list of interface names from ptp4l conf (non-global section headers)."""
    ifaces = []
    try:
        with open(conf_path) as f:
            for line in f:
                s = line.strip()
                if s.startswith('[') and s.endswith(']') and s[1:-1] != 'global':
                    ifaces.append(s[1:-1])
    except OSError:
        pass
    return ifaces


def _instance_type_from_config(conf_path):
    """Read instance-type from a saved config file (best effort)."""
    try:
        with open(conf_path, "r") as f:
            for line in f:
                m = re.match(r'\s*clockType\s+(\S+)', line)
                if m:
                    ct = m.group(1).upper()
                    if ct == "P2P_TC":
                        return "p2p-tc"
                    if ct == "E2E_TC":
                        return "e2e-tc"
                    if ct == "BOUNDARY_CLOCK":
                        return "bc"
        # Default: if more than one port, bc; otherwise oc
        # (approximation — proper detection requires DEFAULT_DATA_SET numberPorts)
    except OSError:
        pass
    return None


def _build_instance(idx, conf_path):
    """Build one instance dict from pmc queries for instance index idx."""
    inst = {"instance-index": idx}

    # default-ds
    dd = _pmc_get_one(conf_path, "DEFAULT_DATA_SET")
    if dd:
        dds = _build_default_ds(dd)
        # Derive instance-type from numberPorts + config file
        num_ports = int(dd.get("numberPorts", "0") or "0")
        it = _instance_type_from_config(conf_path)
        if it is None:
            it = "bc" if num_ports > 1 else "oc"
        dds["instance-type"] = it
        inst["default-ds"] = dds

    # current-ds
    cd = _pmc_get_one(conf_path, "CURRENT_DATA_SET")
    if cd:
        cds = _build_current_ds(cd)
        if cds:
            inst["current-ds"] = cds

    # parent-ds
    pd = _pmc_get_one(conf_path, "PARENT_DATA_SET")
    if pd:
        pds = _build_parent_ds(pd)
        if pds:
            inst["parent-ds"] = pds

    # time-properties-ds
    tp = _pmc_get_one(conf_path, "TIME_PROPERTIES_DATA_SET")
    if tp:
        tpds = _build_time_properties_ds(tp)
        if tpds:
            inst["time-properties-ds"] = tpds

    # ports: PORT_DATA_SET returns one block per port
    port_blocks  = _pmc_get(conf_path, "PORT_DATA_SET")
    stats_blocks = _pmc_get(conf_path, "PORT_STATS_NP")
    ifaces       = _port_interfaces(conf_path)

    # Build a stats map keyed by portIdentity for quick lookup
    stats_by_id = {}
    for sb in stats_blocks:
        pid = sb.get("portIdentity")
        if pid:
            stats_by_id[pid] = _build_port_stats(sb)

    ports = []
    for i, pb in enumerate(port_blocks, start=1):
        pid_raw = pb.get("portIdentity", "")
        port_entry = {}

        # port-index = port number from portIdentity
        pid_dict = _fmt_port_identity(pid_raw)
        port_entry["port-index"] = pid_dict.get("port-number", i)

        if i <= len(ifaces):
            port_entry["underlying-interface"] = ifaces[i - 1]

        pds = _build_port_ds(pb)
        if pds:
            port_entry["port-ds"] = pds

        # 802.1AS port-statistics-ds
        stats = stats_by_id.get(pid_raw)
        if stats:
            port_entry["ieee802-dot1as-gptp:port-statistics-ds"] = stats

        ports.append(port_entry)

    if ports:
        inst["ports"] = {"port": ports}

    return inst


# ---------------------------------------------------------------------------
# Top-level entry point
# ---------------------------------------------------------------------------

def operational():
    """Return operational data for ieee1588-ptp-tt."""
    out = {}
    instances = []

    conf_files = sorted(glob.glob("/etc/linuxptp/ptp4l-*.conf"))
    for conf_path in conf_files:
        m = re.search(r'ptp4l-(\d+)\.conf$', conf_path)
        if not m:
            continue
        idx = int(m.group(1))

        # Only include instances with a live UDS socket (i.e. ptp4l running)
        uds_path = f"/var/run/ptp4l-{idx}"
        if not HOST.exists(uds_path):
            continue

        try:
            inst = _build_instance(idx, conf_path)
            instances.append(inst)
        except Exception as e:
            LOG.debug("ptp4l-%d: skipping instance: %s", idx, e)

    if instances:
        insert(out, "ieee1588-ptp-tt:ptp", "instances", "instance", instances)

    return out
