"""
QoS helpers: capabilities, the rendered scheduler, per-class counters,
and traffic marked the way the tests need it.

Traffic class numbering follows IEEE 802.1Q: class 0 is the lowest.  The
tc ets qdisc numbers its bands the other way, so everything here talks
in traffic classes and hides that.
"""
import json
import re
import subprocess
import time

from infamy.util import until

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

ETS_QUANTUM_UNIT = 1514        # one frame per percent of bandwidth, as rendered


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


def dscp_map(**prio):
    """A custom DSCP map: dscp_map(**{"0": 7, "46": 5}) marks DSCP 0 as
    priority 7 and DSCP 46 as priority 5.  Unlisted codepoints fall to
    the port's default priority.

    Classification at the ingress port is what decides the queue all the
    way to the listener, on one chip as across a cascade or a network of
    switches, so the tests steer their flows here rather than with a
    class table on the egress port.
    """
    return {"entry": [{"dscp": int(dscp), "priority": p} for dscp, p in prio.items()]}


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
    """The ets qdisc: the root, or the child of a tbf root"""
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


def supported_pmd_types(target, port):
    """PMD types the port can negotiate, empty when it has no PHY"""
    data = target.get_data(f"/ietf-interfaces:interfaces/interface[name='{port}']")
    for iface in data["interfaces"]["interface"]:
        eth = iface.get("ieee802-ethernet-interface:ethernet") or iface.get("ethernet") or {}
        return eth.get("infix-ethernet-interface:supported-pmd-types", [])
    return []


def slow_port(target, ssh, port, until):
    """Make the port the bottleneck, return the rate it drains in bit/s

    A port with a PHY that can do 100BASE-TX is negotiated down to it,
    so the queues fill against a real link.  A port without a PHY, as on
    a virtual rig, gets a 10 Mbit/s rate limit instead, which is what
    puts its queues under load.  Returns 0 when neither is possible.
    """
    pmd = "ieee802-ethernet-phy-type:pmd-type-100BASE-TX"
    if pmd in supported_pmd_types(target, port):
        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": port,
                    "ethernet": {"auto-negotiation": {
                        "infix-ethernet-interface:advertised-pmd-types": [pmd]}}
                }]
            }
        }})

        def linked():
            out = ssh.runsh(f"ip -j link show {port}").stdout
            link = json.loads(out or "[]")
            return link and "LOWER_UP" in link[0].get("flags", []) and scheduler(ssh, port)

        until(linked, attempts=60)
        print(ssh.runsh(f"ethtool {port} | grep -i speed").stdout.strip())
        return 100_000_000

    if not supported_pmd_types(target, port):
        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": port,
                    "infix-interfaces:qos": {"egress": {"rate-limit": {"rate": 10_000_000}}}
                }]
            }
        }})
        until(lambda: (root_qdisc(ssh, port) or {}).get("kind") == "tbf")
        show_shaper(ssh, port)
        return 10_000_000

    return 0


def dscp_name_to_num(name):
    """dcb prints DSCP by name when it knows one: CS1, AF21, EF ..."""
    if name.startswith("CS"):
        return int(name[2:]) * 8
    if name.startswith("AF"):
        return int(name[2]) * 8 + int(name[3]) * 2
    if name == "EF":
        return 46
    return int(name)


def dscp_prio(ssh, port, dscp):
    """The priority the port classifies a DSCP to, or None when unmapped

    Read from the DCB table on a port whose driver has one, otherwise
    from the flower rules of the software classifier.
    """
    out = ssh.runsh(f"dcb app show dev {port} 2>/dev/null").stdout
    for line in out.splitlines():
        name, _, rest = line.partition(" ")
        if name.rstrip(":") != "dscp-prio":
            continue
        for token in rest.split():
            key, _, prio = token.partition(":")
            if dscp_name_to_num(key) == dscp:
                return int(prio)
        return None

    out = ssh.runsh(f"tc -j filter show dev {port} ingress").stdout
    for flt in json.loads(out or "[]"):
        opts = flt.get("options") or {}
        tos = str(opts.get("keys", {}).get("ip_tos", ""))
        if not tos or int(tos.split("/")[0], 0) >> 2 != dscp:
            continue
        for act in opts.get("actions", []):
            if act.get("kind") == "skbedit" and "priority" in act:
                prio = str(act["priority"])
                return 0 if prio == "none" else int(prio.rsplit(":", 1)[-1] or "0", 16)
    return None


def show_offload(target, ssh, port, dsa):
    """Log what the fabric took after a scheduler change: the offload list
    and the switch driver's recent messages.  On a switch port the
    scheduler must be offloaded, or the measurement is meaningless"""
    if dsa:
        until(lambda: "transmission-selection" in offload(target, port))
    print(f"{port} offload: {offload(target, port)}")
    log = ssh.runsh("sudo dmesg | grep -i 'mv88e6xxx\\|dsa' | tail -5").stdout.strip()
    if log:
        print(log)


def show_shaper(ssh, port):
    """Log what the rate limit became: the root qdisc with its offloaded
    flag, and on a switch port the port registers, where a shaper the
    driver took shows up as the egress rate control words"""
    print(json.dumps(root_qdisc(ssh, port)))
    print(ssh.runsh(f"ethtool {port} | grep -i speed").stdout.strip())
    if "DEVTYPE=dsa" in ssh.runsh(f"cat /sys/class/net/{port}/uevent").stdout.split():
        regs = ssh.runsh(f"sudo ethtool -d {port}").stdout.strip()
        print("\n".join(regs.splitlines()[:16]))


class Flow:
    """One iperf3 UDP flow, talker to listener, told apart by its port"""
    def __init__(self, name, port, tos, rate_bps, seconds=4, size=1000):
        self.name, self.port, self.tos = name, port, tos
        self.rate, self.seconds, self.size = rate_bps, seconds, size
        self.result = None

    def share(self, total):
        return 100.0 * self.result["packets"] / total if total else 0.0


def run_flows(talker, listener, dst, flows):
    """Run the flows at once, one server and one client process each

    Fills in flow.result from the listener side: bytes and packets
    received, lost packets and the loss in percent.  Returns the total
    datagrams received across the flows.
    """
    servers = {}
    for f in flows:
        servers[f.name] = listener.popen(["iperf3", "-s", "-1", "--json", "-p", str(f.port)],
                                         stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                         text=True)
    time.sleep(1)

    clients = {}
    for f in flows:
        clients[f.name] = talker.popen(["iperf3", "-c", dst, "-p", str(f.port), "-u",
                                        "-b", str(f.rate), "-t", str(f.seconds),
                                        "-l", str(f.size), "--tos", str(f.tos), "--json"],
                                       stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                       text=True)

    offered = {}
    for f in flows:
        out, err = clients[f.name].communicate(timeout=f.seconds + 30)
        if clients[f.name].returncode:
            print(f"{f.name}: iperf3 client failed: {err.strip() or out.strip()[:200]}")
        offered[f.name] = json.loads(out)["end"]["sum"]["packets"]

    total = 0
    for f in flows:
        out, err = servers[f.name].communicate(timeout=30)
        end = json.loads(out)["end"]
        got = end.get("sum_received") or end["sum"]
        # iperf3's receiver keeps its packet count as the highest sequence
        # number seen, so it hides every loss but a tail drop.  The byte
        # count is what arrived, and datagrams are one size, so count
        # those; offered minus received is the honest loss.
        sent = offered[f.name]
        received = got["bytes"] // f.size
        lost = max(sent - received, 0)
        f.result = {"bytes": got["bytes"], "packets": received,
                    "offered": sent, "lost": lost,
                    "lost_percent": 100.0 * lost / sent if sent else 0.0}
        total += f.result["packets"]
        print(f"{f.name}: {f.result['packets']}/{sent} datagrams through, "
              f"{lost} lost ({f.result['lost_percent']:.1f}%)")

    return total
