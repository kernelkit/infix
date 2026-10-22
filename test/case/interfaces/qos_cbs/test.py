#!/usr/bin/env python3
"""
QoS Credit-Based Shaper

Reserve 10 Mbit/s for the top traffic class with the IEEE 802.1Qav
credit-based shaper and offer three times that through it: what
arrives at the listener must be the reservation, within a fifth, since
the shaper bounds the class to its idle slope.  The port reports the
reservation back as the operational idle slope, and that the shaper
runs in the fabric where it forwards in hardware.

Turning the shaper on takes two nodes in two modules, the algorithm on
the traffic class and the idle slope in the cbsa table, and one without
the other is rejected.  Skipped on a product without the TSN modules,
and on a switch fabric whose driver does not offload the shaper.
"""
import infamy
import infamy.qos as qos
from infamy.util import until

RESERVE = 10_000_000           # bit/s, the idle slope
EF = 46 << 2                   # TOS byte of the flow
SIZE = 1000                    # UDP payload, 1042 bytes on the wire
TOLERANCE = 0.2
CBS = "ieee802-dot1q-types:credit-based-shaper"


def config(td0, td1, top, egress, cbsa):
    return {"ietf-interfaces": {
        "interfaces": {
            "interface": [
                {"name": "br0", "type": "infix-if-type:bridge", "enabled": True},
                {
                    "name": td0,
                    "enabled": True,
                    "infix-interfaces:bridge-port": {"bridge": "br0"},
                    "infix-interfaces:qos": {
                        "ingress": {"trust": "dscp",
                                    "dscp-map": qos.dscp_map(**{"0": 0, "46": 7})}
                    }
                },
                {
                    "name": td1,
                    "enabled": True,
                    "infix-interfaces:bridge-port": {"bridge": "br0"},
                    "infix-interfaces:qos": {"egress": egress},
                    "ieee802-dot1dc-cbsa-if:cbsa": cbsa,
                },
            ]
        }
    }}


def must_reject(target, cfg, what, reason):
    try:
        target.put_config_dicts(cfg)
    except Exception as err:
        text = getattr(getattr(err, "response", None), "text", None) or str(err)
        assert reason in text, f"{what} rejected for another reason:\n{text}"
        print(f"Rejected as expected: {reason}")
        return
    raise AssertionError(f"{what} was accepted")


def oper_idle_slope(target, port, tc):
    data = target.get_data(f"/ietf-interfaces:interfaces/interface[name='{port}']")
    iface = next(iter(data["interfaces"]["interface"]), {})
    cbsa = iface.get("ieee802-dot1dc-cbsa-if:cbsa") or iface.get("cbsa") or {}
    for entry in cbsa.get("cbsa-parameter-table", []):
        if entry["traffic-class"] == tc:
            return int(entry.get("oper-idle-slope", 0))
    return None


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        _, td0 = env.ltop.xlate("target", "data1")
        _, td1 = env.ltop.xlate("target", "data2")
        _, hd0 = env.ltop.xlate("host", "data1")
        _, hd1 = env.ltop.xlate("host", "data2")
        port = infamy.capability.Port(target, td1, tgtssh)
        if not target.has_model("ieee802-dot1dc-cbsa-if"):
            test.skip("the product has no credit-based shaper model")
        top = port.traffic_classes - 1

    with test.step("Reject a credit-based-shaper class without a reservation"):
        must_reject(target, config(td0, td1, top,
                                   {"traffic-class": [{"id": top, "algorithm": CBS}]}, {}),
                    "credit-based-shaper without admin-idle-slope",
                    "needs an admin-idle-slope")

    with test.step("Reject a reservation on a class that is not credit-based-shaper"):
        must_reject(target, config(td0, td1, top, {},
                                   {"cbsa-parameter-table": [
                                       {"traffic-class": top, "admin-idle-slope": RESERVE}]}),
                    "admin-idle-slope on a strict class",
                    "algorithm is not credit-based-shaper")

    with test.step("Bridge the two ports, reserve 10 Mbit/s for the top class on the egress port"):
        target.put_config_dicts(config(
            td0, td1, top,
            {"traffic-class": [{"id": top, "algorithm": CBS}]},
            {"cbsa-parameter-table": [{"traffic-class": top, "admin-idle-slope": RESERVE}]}))
        until(lambda: any(q.get("kind") == "cbs" for q in qos.qdiscs(tgtssh, td1)))
        qos.show_offload(port, "credit-based-shaper")

        port.require_offload(test, "credit-based-shaper")

    with test.step("Verify the port reports the reservation as its operational idle slope"):
        until(lambda: oper_idle_slope(target, td1, top) == RESERVE)

    with infamy.IsolatedMacVlan(hd0) as ns0, \
         infamy.IsolatedMacVlan(hd1) as ns1:

        with test.step("Set up the talker and listener namespaces"):
            ns0.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.1/24 dev iface")
            ns1.runsh("set -ex; ip link set iface up; ip addr add 192.168.20.2/24 dev iface")
            ns0.must_reach("192.168.20.2")

        with test.step("One EF flow in the shaped class at three times the reservation, expect the reservation through"):
            flow = qos.Flow("EF", 5201, EF, 3 * RESERVE, size=SIZE)
            qos.run_flows(ns0, ns1, "192.168.20.2", [flow])
            expect = RESERVE * SIZE / (SIZE + 42)
            print(f"EF: {flow.throughput() / 1e6:.2f} Mbit/s through, "
                  f"reservation lets {expect / 1e6:.2f}")
            assert abs(flow.throughput() - expect) <= TOLERANCE * expect, \
                f"EF: {flow.throughput() / 1e6:.2f} Mbit/s through a " \
                f"{RESERVE / 1e6:.0f} Mbit/s reservation"

    test.succeed()
