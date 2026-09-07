#!/usr/bin/env python3
"""
QoS Defaults Out of the Box

An interface without any qos configuration is fully specified by the
model's defaults, and every physical port runs them from boot:

 - trust PCP, the tag of VLAN-tagged frames, with default priority 0
   for everything else
 - one traffic class per transmit queue, eight on a single-queue port,
   mapped per IEEE 802.1Q-2022 Table 8-5 with strict priority throughout

Verify that the running configuration carries no qos container, that the
class count follows the queue count rule, that the root qdisc carries the
Table 8-5 map for it, and that classification is in place: in the DCB
tables on a port whose driver has them, as tc flower rules otherwise.
"""
import json
import infamy
from infamy.util import until

TABLE_8_5 = {
    2: [0, 0, 0, 0, 1, 1, 1, 1],
    3: [0, 0, 0, 0, 1, 1, 2, 2],
    4: [0, 0, 1, 1, 2, 2, 3, 3],
    5: [0, 0, 1, 1, 2, 2, 3, 4],
    6: [1, 0, 2, 2, 3, 3, 4, 5],
    7: [1, 0, 2, 3, 4, 4, 5, 6],
    8: [1, 0, 2, 3, 4, 5, 6, 7],
}


def capabilities(target, port):
    data = target.get_data(f"/ietf-interfaces:interfaces/interface[name='{port}']"
                           "/infix-interfaces:qos/capabilities")
    for iface in data["interfaces"]["interface"]:
        qos = iface.get("qos") or iface.get("infix-interfaces:qos") or {}
        return qos.get("capabilities", {})
    return {}


def running_qos(target, port):
    running = target.get_config_dict("/ietf-interfaces:interfaces")
    for iface in running["interfaces"]["interface"]:
        if iface["name"] == port:
            return iface.get("qos") or iface.get("infix-interfaces:qos")
    return None


def root_qdisc(ssh, port):
    out = ssh.runsh(f"tc -j qdisc show dev {port}").stdout
    for qdisc in json.loads(out or "[]"):
        if qdisc.get("root"):
            return qdisc
    return None


def tx_queues(ssh, port):
    out = ssh.runsh(f"ls /sys/class/net/{port}/queues").stdout
    return len([q for q in out.split() if q.startswith("tx-")])


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        _, port = env.ltop.xlate("target", "data")

    with test.step("Verify the running configuration has no qos container"):
        assert running_qos(target, port) is None, f"{port} has qos configuration"

    with test.step("Verify the class count is the queue count, or eight for one queue"):
        caps = capabilities(target, port)
        queues = tx_queues(tgtssh, port)
        expected = min(queues, 8) if queues > 1 else 8
        print(f"{port}: {queues} tx queues, capabilities {caps}")
        assert caps.get("max-traffic-classes", 8) == expected, caps
        num_tc = expected

    with test.step("Verify the root qdisc carries the Table 8-5 map, all strict"):
        def default_table():
            qdisc = root_qdisc(tgtssh, port)
            if not qdisc:
                return False
            opts = qdisc.get("options", {})
            if qdisc["kind"] == "mqprio":
                return opts.get("map", [])[:8] == TABLE_8_5[num_tc]
            if qdisc["kind"] == "ets":
                return (opts.get("bands") == num_tc and opts.get("strict") == num_tc and
                        opts.get("priomap", [])[:8] == [num_tc - 1 - tc for tc in TABLE_8_5[num_tc]])
            return False
        until(default_table)
        print(json.dumps(root_qdisc(tgtssh, port)))

    with test.step("Verify classification trusts PCP by default"):
        if caps.get("supported-trust-order"):
            out = tgtssh.runsh(f"dcb apptrust show dev {port}").stdout
            assert out.replace(":", "").split() == ["order", "pcp"], out
        else:
            out = tgtssh.runsh(f"tc -j filter show dev {port} ingress").stdout
            keys = [f["options"].get("keys", {}) for f in json.loads(out or "[]") if f.get("options")]
            assert sum(1 for k in keys if "vlan_prio" in k) == 8, keys
            assert not any("ip_tos" in k for k in keys), keys

    test.succeed()
