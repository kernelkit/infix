#!/usr/bin/env python3
"""
QoS Traffic Classes and Transmission Selection

Configure a custom priority to traffic class map on a port with the two
lowest classes sharing 67:33 and the rest strict priority, and verify the
root qdisc reflects it: an ets qdisc with the strict bands first, the
shares as quanta of one frame per percent, and the priomap inverted as
802.1Q numbering requires.  A driver that offloads it marks the qdisc
offloaded.

The port's class count comes from its transmit queues, or eight for a
single-queue port.  Switching the table to the ieee-sr preset must put
the SR classes, priorities 2 and 3, on the two highest classes.  A rate
limit must put a tbf root qdisc above the scheduler, with the same ets
table beneath it, and go away with the setting.  Removing the qos
container must restore the default table, IEEE 802.1Q-2022 Table 8-5,
and the operational datastore must report whether transmission
selection is offloaded throughout.
"""
import json
import infamy
from infamy.util import until

# IEEE 802.1Q-2022 Table 8-5 and Table 34-1, indexed by class count
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


def qos_xpath(port, path=""):
    return f"/ietf-interfaces:interfaces/interface[name='{port}']/infix-interfaces:qos{path}"


def root_qdisc(ssh, port):
    """Return the root qdisc of port as a dict, or None"""
    out = ssh.runsh(f"tc -j qdisc show dev {port}").stdout
    for qdisc in json.loads(out or "[]"):
        if qdisc.get("root"):
            return qdisc
    return None


def qdisc_matches(qdisc, num_tc, prio_map, strict, quanta):
    """Check a root qdisc against the expected 802.1Q table"""
    if not qdisc:
        return False
    opts = qdisc.get("options", {})
    if qdisc["kind"] == "ets":
        return (opts.get("bands") == num_tc and opts.get("strict") == strict
                and opts.get("quanta", []) == quanta
                and opts.get("priomap", [])[:8] == [num_tc - 1 - tc for tc in prio_map])
    return False


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        _, port = env.ltop.xlate("target", "data")

        cap = infamy.capability.Port(target, port, tgtssh)
        num_tc = cap.traffic_classes
        print(f"{port}: {num_tc} traffic classes")
        assert num_tc and 2 <= num_tc <= 8, f"max-traffic-classes {num_tc}"
        offloaded = cap.offloads("transmission-selection")

    with test.step("Configure a custom map, the two lowest classes sharing 67:33"):
        # Table 8-5 with the two lowest classes swapped, so the map is
        # visibly custom on any class count
        custom = [1 if tc == 0 else 0 if tc == 1 else tc for tc in TABLE_8_5[num_tc]]
        table = {f"priority{prio}": tc for prio, tc in enumerate(custom)}

        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": port,
                    "enabled": True,
                    "infix-interfaces:qos": {
                        "egress": {
                            "traffic-class-table": table,
                            "traffic-class": [
                                {"id": 1, "algorithm": "ieee802-dot1q-types:enhanced-transmission-selection",
                                 "bandwidth": 67},
                                {"id": 0, "algorithm": "ieee802-dot1q-types:enhanced-transmission-selection",
                                 "bandwidth": 33},
                            ]
                        }
                    }
                }]
            }
        }})

    with test.step("Verify the root qdisc carries the custom map and shares"):
        until(lambda: qdisc_matches(root_qdisc(tgtssh, port), num_tc, custom, num_tc - 2,
                                    [67 * 1514, 33 * 1514]))
        print(json.dumps(root_qdisc(tgtssh, port)))

    with test.step("Verify the offload capability matches the qdisc"):
        taken = root_qdisc(tgtssh, port).get("offloaded", False)
        until(lambda: cap.offloads("transmission-selection") == taken)

    with test.step("Switch to the ieee-sr preset and verify SR classes on top"):
        target.delete_xpath(qos_xpath(port, "/egress"))
        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": port,
                    "infix-interfaces:qos": {
                        "egress": {"traffic-class-table": {"preset": "ieee-sr"}}
                    }
                }]
            }
        }})
        until(lambda: qdisc_matches(root_qdisc(tgtssh, port), num_tc, TABLE_34_1[num_tc], num_tc, []))

    with test.step("Add a 10 Mbit/s rate limit and verify the tbf root above the ets table"):
        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": port,
                    "infix-interfaces:qos": {
                        "egress": {"rate-limit": {"rate": 10000000}}
                    }
                }]
            }
        }})

        def rate_limited():
            out = tgtssh.runsh(f"tc -j qdisc show dev {port}").stdout
            qdiscs = json.loads(out or "[]")
            root = [q for q in qdiscs if q.get("root")]
            child = [q for q in qdiscs if q.get("parent") == "1:1"]
            return (root and root[0]["kind"] == "tbf" and root[0]["options"]["rate"] == 1250000
                    and child and qdisc_matches(child[0], num_tc, TABLE_34_1[num_tc], num_tc, []))
        until(rate_limited)
        print(tgtssh.runsh(f"tc -j qdisc show dev {port}").stdout)

    with test.step("Remove the rate limit and verify the scheduler is the root again"):
        target.delete_xpath(qos_xpath(port, "/egress/rate-limit"))
        until(lambda: qdisc_matches(root_qdisc(tgtssh, port), num_tc, TABLE_34_1[num_tc], num_tc, []))

    with test.step("Remove qos configuration and verify the default table is back"):
        target.delete_xpath(qos_xpath(port))
        until(lambda: qdisc_matches(root_qdisc(tgtssh, port), num_tc, TABLE_8_5[num_tc], num_tc, []))
        assert cap.offloads("transmission-selection") == offloaded

    test.succeed()
