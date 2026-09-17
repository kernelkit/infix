"""
QoS helpers: capabilities, the rendered scheduler, per-class counters,
and traffic marked the way the tests need it.

Traffic class numbering follows IEEE 802.1Q: class 0 is the lowest.  The
tc ets qdisc numbers its bands the other way and mqprio has classes of
its own, so everything here talks in traffic classes and hides both.
"""
import json
import re

# IEEE 802.1Q-2022 Table 8-5 and Table 34-1, priority to traffic class,
# indexed by the number of classes
TABLE_8_5 = {
    2: [0, 0, 0, 0, 1, 1, 1, 1],
    3: [0, 0, 0, 0, 1, 1, 2, 2],
    4: [0, 0, 1, 1, 2, 2, 3, 3],
    5: [0, 0, 1, 1, 2, 2, 3, 4],
    6: [1, 0, 2, 2, 3, 3, 4, 5],
    7: [1, 0, 2, 3, 4, 4, 5, 6],
    8: [1, 0, 2, 3, 4, 5, 6, 7],
}
TABLE_34_1 = {
    2: [0, 0, 1, 1, 0, 0, 0, 0],
    3: [0, 0, 1, 2, 0, 0, 0, 0],
    4: [0, 0, 2, 3, 1, 1, 1, 1],
    5: [0, 0, 3, 4, 1, 1, 2, 2],
    6: [0, 0, 4, 5, 1, 1, 2, 3],
    7: [0, 0, 5, 6, 1, 2, 3, 4],
    8: [1, 0, 6, 7, 2, 3, 4, 5],
}

STRICT = "ieee802-dot1q-types:strict-priority"
ETS = "ieee802-dot1q-types:enhanced-transmission-selection"

# mqprio addresses its traffic classes from this minor number upwards
MQPRIO_TC_BASE = 0xffe0


def xpath(port, path=""):
    return f"/ietf-interfaces:interfaces/interface[name='{port}']/infix-interfaces:qos{path}"


def capabilities(target, port):
    """The port's qos/capabilities container from the operational datastore"""
    data = target.get_data(xpath(port, "/capabilities"))
    for iface in data["interfaces"]["interface"]:
        qos = iface.get("qos") or iface.get("infix-interfaces:qos") or {}
        return qos.get("capabilities", {})
    return {}


def num_classes(target, port):
    return capabilities(target, port).get("max-traffic-classes", 8)


def offload(target, port):
    return capabilities(target, port).get("offload", [])


def traffic_classes(weighted, strict=()):
    """The traffic-class list for a layout: weighted is {class: percent}

    The shares are TCBandwidth percentages and must sum to 100, so 2:1
    is {1: 67, 0: 33}.
    """
    classes = [{"id": tc, "algorithm": STRICT} for tc in strict]
    for tc, share in weighted.items():
        classes.append({"id": tc, "algorithm": ETS, "bandwidth": share})
    return classes


def qdiscs(ssh, port):
    out = ssh.runsh(f"tc -j qdisc show dev {port}").stdout
    return json.loads(out or "[]")


def root_qdisc(ssh, port):
    for qdisc in qdiscs(ssh, port):
        if qdisc.get("root"):
            return qdisc
    return None


def scheduler(ssh, port):
    """The ets or mqprio qdisc: the root, or the child of a tbf root"""
    root = root_qdisc(ssh, port)
    if root and root["kind"] == "tbf":
        for qdisc in qdiscs(ssh, port):
            if qdisc.get("parent") == "1:1":
                return qdisc
        return None
    return root


def scheduler_matches(qdisc, num_tc, prio_map, strict=None, quanta=None):
    """Check a scheduler qdisc against a priority to class map and layout

    strict is the number of strict classes and quanta the shares of the
    others from the top down; either left as None is not checked.
    """
    if not qdisc:
        return False
    opts = qdisc.get("options", {})
    if qdisc["kind"] == "mqprio":
        return opts.get("map", [])[:8] == list(prio_map)
    if qdisc["kind"] == "ets":
        if opts.get("bands") != num_tc:
            return False
        if opts.get("priomap", [])[:8] != [num_tc - 1 - tc for tc in prio_map]:
            return False
        if strict is not None and opts.get("strict") != strict:
            return False
        if quanta is not None and opts.get("quanta", []) != list(quanta):
            return False
        return True
    return False


def class_stats(ssh, port, num_tc):
    """Per traffic class counters of the scheduler: {tc: {packets, bytes, drops}}

    Read from tc -s class show.  iproute2 6.14 renders classes as JSON,
    older releases ignore -j for classes and print text, so both are
    parsed.
    """
    out = ssh.runsh(f"tc -s -j class show dev {port}").stdout.strip()
    stats = {}

    def tc_of(kind, handle):
        minor = int(handle.split(":")[1], 16)
        if kind == "ets":
            return num_tc - minor         # band 0, minor 1, is the top class
        if kind == "mqprio" and minor >= MQPRIO_TC_BASE:
            return minor - MQPRIO_TC_BASE
        return None

    if out.startswith("["):
        for cls in json.loads(out):
            tc = tc_of(cls.get("class"), cls.get("handle", "0:0"))
            if tc is not None:
                st = cls.get("stats", cls)   # tc 6.14 nests the counters
                stats[tc] = {"packets": st.get("packets", 0),
                             "bytes": st.get("bytes", 0),
                             "drops": st.get("drops", 0)}
        return stats

    current = None
    for line in out.splitlines():
        head = re.match(r"class (\S+) (\S+)", line)
        if head:
            current = tc_of(head.group(1), head.group(2))
            continue
        sent = re.match(r"\s*Sent (\d+) bytes (\d+) pkt \(dropped (\d+)", line)
        if sent and current is not None:
            stats[current] = {"bytes": int(sent.group(1)), "packets": int(sent.group(2)),
                              "drops": int(sent.group(3))}
    return stats


def stats_delta(before, after):
    return {tc: {k: after[tc][k] - before.get(tc, {}).get(k, 0) for k in after[tc]}
            for tc in after}


def neighbour_mac(ns, ip):
    """MAC of ip from the namespace's neighbour table, None if unresolved"""
    out = ns.runsh(f"ip -j neigh show {ip}").stdout
    for entry in json.loads(out or "[]"):
        if entry.get("lladdr"):
            return entry["lladdr"]
    return None


def mausezahn(ns, iface, src_ip, dst_ip, dst_mac, count=10, delay="5msec",
              vid=None, pcp=None, dscp=None, dport=7777):
    """Send count UDP datagrams from the namespace with an exact marking

    vid and pcp add a VLAN tag with that PCP; dscp sets the IP DSCP.
    mausezahn writes the frame itself, so the source address is given
    rather than taken from a VLAN device it knows nothing about.
    """
    cmd = ["mausezahn", iface, "-c", str(count), "-d", delay,
           "-A", src_ip, "-B", dst_ip, "-b", dst_mac]
    if vid is not None:
        cmd += ["-Q", f"{pcp or 0}:{vid}"]
    params = f"dp={dport}"
    if dscp is not None:
        params += f",dscp={dscp}"
    cmd += ["-t", "udp", params]
    return ns.run(cmd, check=True, text=True, capture_output=True)
