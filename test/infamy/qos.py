"""
QoS helpers: capabilities, the scheduler qdisc, per-class counters,
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

ETS_QUANTUM_UNIT = 1514        # one frame per percent of bandwidth, as configured
WIRE_OVERHEAD = 20             # preamble, SFD and interframe gap
WIRE_MAX = 1522 + WIRE_OVERHEAD   # tagged 1500-byte frame on the wire


def xpath(port, path=""):
    return f"/ietf-interfaces:interfaces/interface[name='{port}']/infix-interfaces:qos{path}"


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


def tx_rate(ssh, port, seconds=3):
    """Bit rate a port transmits at, from its byte counter over @seconds"""
    stat = f"/sys/class/net/{port}/statistics/tx_bytes"
    out = ssh.runsh(f"a=$(cat {stat}); sleep {seconds}; echo $(($(cat {stat}) - a))")
    return int(out.stdout) * 8 / seconds


def qdiscs(ssh, port):
    out = ssh.runsh(f"tc -j qdisc show dev {port}").stdout
    return json.loads(out or "[]")


def guard_band(speed):
    """Nanoseconds a maximum-size frame takes at @speed bps: how long a
    gate control list keeps all gates closed before a protected window,
    so no frame started before it can overrun into it"""
    return -(-WIRE_MAX * 8 * 1_000_000_000 // speed)


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

    Read from tc -s class show.  iproute2 6.14 prints classes as JSON,
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


def slow_port(port, until):
    """Make the port the bottleneck, return the rate it drains in bps

    A port with a PHY that can do 100BASE-TX is negotiated down to it,
    so the queues fill against a real link.  A port without a PHY, as on
    a virtual rig, is shaped to 10 Mbps instead, which is what
    puts its queues under load.  Returns 0 when neither is possible.
    """
    pmd = "ieee802-ethernet-phy-type:pmd-type-100BASE-TX"
    pmds = supported_pmd_types(port.target, port.name)

    if pmd in pmds:
        port.target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": port.name,
                    "ethernet": {"auto-negotiation": {
                        "infix-ethernet-interface:advertised-pmd-types": [pmd]}}
                }]
            }
        }})

        def linked():
            out = port.ssh.runsh(f"ip -j link show {port.name}").stdout
            link = json.loads(out or "[]")
            return (link and "LOWER_UP" in link[0].get("flags", [])
                    and scheduler(port.ssh, port.name))

        until(linked)
        print(port.ssh.runsh(f"ethtool {port.name} | grep -i speed").stdout.strip())
        return 100_000_000

    if not pmds and not (port.switched and "egress-shaper" not in port.supported):
        port.target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": port.name,
                    "infix-interfaces:qos": {"egress-shaper": {"rate": 10_000_000}}
                }]
            }
        }})
        until(lambda: (root_qdisc(port.ssh, port.name) or {}).get("kind") == "tbf")
        show_shaper(port)
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


def show_offload(port, stage="transmission-selection"):
    """Log what the fabric took after a change: the offload list and the
    driver's recent complaints.  A fabric port must still run @stage in
    hardware, or the measurement that follows means nothing
    """
    if port.switched:
        until(lambda: port.offloads(stage))
    print(f"{port.name} offload: {port.offload}")

    if not port.driver:
        return

    log = port.ssh.runsh(f"sudo dmesg | grep -i '{port.driver}' | tail -5").stdout.strip()
    if log:
        print(log)


def show_shaper(port):
    """Log what the egress shaper became: the root qdisc with its offloaded
    flag, and on a fabric port the port registers, where a shaper the
    driver took shows up as the egress rate control words"""
    print(json.dumps(root_qdisc(port.ssh, port.name)))
    print(port.ssh.runsh(f"ethtool {port.name} | grep -i speed").stdout.strip())
    if port.switched:
        regs = port.ssh.runsh(f"sudo ethtool -d {port.name}").stdout.strip()
        print("\n".join(regs.splitlines()[:16]))


class Flow:
    """One iperf3 UDP flow, talker to listener, told apart by its port"""
    def __init__(self, name, port, tos, rate_bps, seconds=4, size=1000):
        self.name, self.port, self.tos = name, port, tos
        self.rate, self.seconds, self.size = rate_bps, seconds, size
        self.result = None

    def throughput(self):
        """Payload bit rate that arrived in the steady state"""
        return self.result["steady"]

    def shortfall(self):
        """Share of the offered rate the port held back in the steady state"""
        return 1.0 - self.throughput() / self.rate

    def require_backlog(self, share=0.1):
        """The port must hold back this share of the flow; one that was
        never full proves nothing"""
        assert self.shortfall() > share, \
            f"{self.name} got {self.throughput() / 1e6:.2f} of the " \
            f"{self.rate / 1e6:.2f} Mbps it offered, the port was never full"

    def share(self, total):
        """Percent of total, the steady-state bit rate of a set of flows"""
        return 100.0 * self.throughput() / total if total else 0.0


def _connected(talker, flows, spread=0.5):
    """Whether the clients' TCP control and UDP data sockets all came up
    within @spread seconds of each other"""
    up = {}
    start = time.monotonic()
    while len(up) < len(flows) and time.monotonic() - start < 5:
        peers = talker.runsh("ss -Htun state established").stdout.split()
        for f in flows:
            if f.port not in up and sum(p.endswith(f":{f.port}") for p in peers) >= 2:
                up[f.port] = time.monotonic()
    return len(up) == len(flows) and max(up.values()) - min(up.values()) <= spread


def _start_flows(talker, listener, dst, flows):
    """One attempt: the servers, and the datagrams each client offered,
    None for a client that failed or connected late"""
    servers = {}
    for f in flows:
        servers[f.name] = listener.popen(["iperf3", "-s", "-1", "--json", "-p", str(f.port)],
                                         stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                         text=True)
    time.sleep(1)

    clients = {}
    for f in sorted(flows, key=lambda f: f.tos):
        clients[f.name] = talker.popen(["iperf3", "-c", dst, "-p", str(f.port), "-u",
                                        "-b", str(f.rate), "-t", str(f.seconds),
                                        "-l", str(f.size), "--tos", str(f.tos), "--json"],
                                       stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                       text=True)
    in_step = _connected(talker, flows)

    offered = {}
    for f in flows:
        out, err = clients[f.name].communicate(timeout=f.seconds + 30)
        sent = json.loads(out or "{}").get("end", {}).get("sum", {}).get("packets")
        if clients[f.name].returncode or sent is None:
            print(f"{f.name}: iperf3 client failed: {err.strip() or out.strip()[:200]}")
        offered[f.name] = sent if in_step else None

    if not in_step:
        print("iperf3 clients connected too far apart to share their middle seconds")
    return servers, offered


def run_flows(talker, listener, dst, flows):
    """Run the flows at once, one server and one client process each

    The listener counts until the talker's end-of-test message reaches
    it over the iperf3 control connection, DSCP 0.  The DUT must
    classify that traffic behind the data, into the lowest class a
    flow loads: the message then follows the last datagram out of the
    port, every datagram the port let through is counted, and the loss
    is what the port dropped.

    That class starves under a strict class the port cannot drain, so
    a client still connecting when the data of another fills the port
    never gets through.  The clients start back to back, lowest marking
    first, and the flows run again when one fails to connect, or connects
    so late that the flows' middle seconds no longer line up.

    The steady state is the middle seconds of a flow, after the queues
    filled and before the talkers stopped.  Shares and throughput are
    measured there, so the queues draining after the talkers stop do
    not count towards them.

    Fills in flow.result from the listener side: datagrams offered and
    received, lost datagrams, the loss in percent and the steady-state
    payload bit rate.  Returns the total steady-state payload bit rate
    across the flows.
    """
    for _ in range(3):
        servers, offered = _start_flows(talker, listener, dst, flows)
        if None not in offered.values():
            break
        for server in servers.values():
            server.kill()
            server.communicate()
    else:
        raise RuntimeError("iperf3 clients failed to connect together three times")

    total = 0.0
    for f in flows:
        out, err = servers[f.name].communicate(timeout=30)
        report = json.loads(out)
        end = report["end"]
        got = end.get("sum_received") or end["sum"]
        # iperf3's receiver keeps its packet count as the highest sequence
        # number seen, so it hides every loss but a tail drop.  The byte
        # count is what arrived, and datagrams are one size, so count
        # those; offered minus received is the honest loss.
        sent = offered[f.name]
        received = got["bytes"] // f.size
        lost = max(sent - received, 0)
        middle = report["intervals"][1:f.seconds - 1]
        steady = 8.0 * sum(iv["sum"]["bytes"] for iv in middle) / len(middle)
        f.result = {"packets": received, "offered": sent, "lost": lost,
                    "lost_percent": 100.0 * lost / sent if sent else 0.0,
                    "steady": steady}
        total += f.throughput()
        print(f"{f.name}: {f.result['packets']}/{sent} datagrams through, "
              f"{lost} lost ({f.result['lost_percent']:.1f}%), "
              f"{f.throughput() / 1e6:.2f} Mbps in the steady state")

    return total


def must_reject(what, func, *reasons):
    """func must fail its commit, and the error must name one of the reasons

    With no reasons any error will do; a rejected delete over RESTCONF
    carries no message, so the test checks the effect instead."""
    try:
        func()
    except Exception as err:
        text = getattr(getattr(err, "response", None), "text", None) or str(err)
        matched = [reason for reason in reasons if reason in text]
        assert matched or not reasons, f"{what} rejected for another reason:\n{text}"
        print(f"Rejected as expected: {matched[0] if matched else what}")
        return
    raise AssertionError(f"{what} was accepted")
