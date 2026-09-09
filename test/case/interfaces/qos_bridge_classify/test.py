#!/usr/bin/env python3
"""
QoS Bridge Classification and Remarking

Bridge two ports and send frames straight through the switch, so that on
a switch chip the traffic never passes the CPU and only the hardware
tables can classify and remark it.  The DSCP and PCP on the wire at the
egress port reveal the priority the ingress port assigned:

 - trust dscp: untagged IP frames on VLAN 20, DSCP 46 (EF) to priority 5,
   DSCP 4 unknown to the preset to the default priority 1
 - trust pcp with a custom map: tagged frames on VLAN 10, PCP 3 to
   priority 6, PCP 0 to priority 4, PCP 7 unmapped to the default
   priority 2
 - remark off: DSCP leaves as it came in, and so does the PCP unless the
   switch fabric encodes it from the priority, which its rewrite table
   then still shows

With remarking on, the egress port writes the class selector of the
priority as DSCP and, where the driver offloads remarking, the priority
as PCP; PCP remarking has no software path, so on other ports only the
DSCP is checked.  The test is skipped on a switch whose driver cannot
offload classification, since frames the fabric forwards never see the
kernel's rules.
"""
import re
import infamy
from infamy.util import until

# DSCP cases with trust dscp: (dscp, expected priority)
DSCP_CASES = ((46, 5), (0, 0), (26, 3), (4, 1))
# PCP cases with trust pcp and a custom map: (pcp, expected priority)
PCP_CASES = ((3, 6), (0, 4), (7, 2))


def capabilities(target, port):
    data = target.get_data(f"/ietf-interfaces:interfaces/interface[name='{port}']"
                           "/infix-interfaces:qos/capabilities")
    for iface in data["interfaces"]["interface"]:
        qos = iface.get("qos") or iface.get("infix-interfaces:qos") or {}
        return qos.get("capabilities", {})
    return {}


def qos_config(target, td0, td1, ingress, remark):
    target.put_config_dicts({"ietf-interfaces": {
        "interfaces": {
            "interface": [
                {"name": td0, "infix-interfaces:qos": {"ingress": ingress}},
                {"name": td1, "infix-interfaces:qos": {"egress": {"remark": remark}}},
            ]
        }
    }})


def parse(packets):
    """Return {icmp id: (vid, pcp, dscp)} from tcpdump -e -v output"""
    seen = {}
    for m in re.finditer(r"vlan (\d+), p (\d+), .*?\(tos 0x([0-9a-f]+).*?"
                         r"ICMP echo request, id (\d+)", packets, re.S):
        seen[int(m.group(4))] = (int(m.group(1)), int(m.group(2)), int(m.group(3), 16) >> 2)
    return seen


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        _, td0 = env.ltop.xlate("target", "data1")
        _, td1 = env.ltop.xlate("target", "data2")
        _, hd0 = env.ltop.xlate("host", "data1")
        _, hd1 = env.ltop.xlate("host", "data2")

        dcb = bool(capabilities(target, td0).get("supported-trust-order"))
        uevent = tgtssh.runsh(f"cat /sys/class/net/{td0}/uevent").stdout
        dsa = "DEVTYPE=dsa" in uevent.split()
        print(f"{td0}: DCB {'supported' if dcb else 'not supported'}, DSA port: {dsa}")
        if dsa and not dcb:
            print("switch forwards in hardware without DCB support, skipping")
            test.skip()

    with test.step("Configure a VLAN bridge, VLAN 10 tagged and VLAN 20 untagged on ingress"):
        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                        "bridge": {
                            "vlans": {
                                "vlan": [
                                    {"vid": 10, "tagged": [td0, td1]},
                                    {"vid": 20, "untagged": [td0], "tagged": [td1]},
                                ]
                            }
                        }
                    },
                    {
                        "name": td0,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {"pvid": 20, "bridge": "br0"},
                        "infix-interfaces:qos": {
                            "ingress": {
                                "trust": "dscp",
                                "default-priority": 1,
                                "dscp-map": {"preset": "ietf"},
                            }
                        }
                    },
                    {
                        "name": td1,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {"bridge": "br0"},
                        "infix-interfaces:qos": {
                            "egress": {"remark": {"pcp": "from-priority", "dscp": "from-priority"}}
                        }
                    },
                ]
            }
        }})

    with infamy.IsolatedMacVlan(hd0) as ns0, \
         infamy.IsolatedMacVlan(hd1) as ns1:

        with test.step("Set up host namespaces, VLAN 10 tagged on both, VLAN 20 untagged on the sender"):
            ns0.runsh("""
            set -ex
            ip link set iface up
            ip addr add 192.168.20.1/24 dev iface
            ip link add dev vlan10 link iface up type vlan id 10
            ip addr add 192.168.10.1/24 dev vlan10
            """)
            ns1.runsh("""
            set -ex
            ip link set iface up
            ip link add dev vlan10 link iface up type vlan id 10
            ip addr add 192.168.10.2/24 dev vlan10
            ip link add dev vlan20 link iface up type vlan id 20
            ip addr add 192.168.20.2/24 dev vlan20
            """)
            ns0.must_reach("192.168.20.2")
            ns0.must_reach("192.168.10.2")

        if dcb:
            with test.step("Verify classification and remarking are offloaded"):
                until(lambda: "classification" in capabilities(target, td0).get("offload", []))
                until(lambda: "remarking" in capabilities(target, td1).get("offload", []))
        remark_hw = "remarking" in capabilities(target, td1).get("offload", [])

        def expect(seen, ident, vid, pcp, dscp, what):
            """Check one captured echo request, PCP only where the driver remarks it"""
            assert ident in seen, f"{what}: no echo request captured"
            got = seen[ident]
            want = (vid, pcp if remark_hw else got[1], dscp)
            assert got == want, f"{what}: got (vid, pcp, dscp) {got}, expected {want}"

        def capture(send):
            pcap = ns1.pcap("icmp[icmptype] == icmp-echo")
            with pcap:
                send()
            packets = pcap.tcpdump("-e -v")
            print(packets)
            return parse(packets)

        def set_pcp(pcp):
            """Tag everything the sender puts on VLAN 10 with this PCP, whatever its TOS"""
            qmap = " ".join(f"{prio}:{pcp}" for prio in range(8))
            ns0.runsh(f"ip link set dev vlan10 type vlan egress-qos-map {qmap}")

        def send_dscp():
            for n, (dscp, _) in enumerate(DSCP_CASES):
                ns0.runsh(f"ping -c1 -w2 -Q {dscp << 2} -e {100 + n} 192.168.20.2")

        def send_pcp():
            for n, (pcp, _) in enumerate(PCP_CASES):
                set_pcp(pcp)
                ns0.runsh(f"ping -c1 -w2 -e {200 + n} 192.168.10.2")

        with test.step("Send untagged IP frames with trust dscp, verify PCP and DSCP from priority"):
            seen = capture(send_dscp)
            for n, (dscp, prio) in enumerate(DSCP_CASES):
                expect(seen, 100 + n, 20, prio, prio << 3, f"DSCP {dscp}")

        with test.step("Switch to trust pcp with a custom map and default priority 2"):
            qos_config(target, td0, td1, {
                "trust": "pcp",
                "default-priority": 2,
                "pcp-map": {
                    "entry": [
                        {"pcp": 3, "dei": "false", "priority": 6},
                        {"pcp": 3, "dei": "true", "priority": 6},
                        {"pcp": 0, "dei": "false", "priority": 4},
                        {"pcp": 0, "dei": "true", "priority": 4},
                    ]
                },
            }, {"pcp": "from-priority", "dscp": "from-priority"})
            ns0.must_reach("192.168.10.2")

        with test.step("Send tagged frames, verify PCP and DSCP from the mapped priority"):
            seen = capture(send_pcp)
            for n, (pcp, prio) in enumerate(PCP_CASES):
                expect(seen, 200 + n, 10, prio, prio << 3, f"PCP {pcp}")

        with test.step("Turn remarking off"):
            qos_config(target, td0, td1, {"trust": "pcp", "default-priority": 2},
                       {"pcp": "none", "dscp": "none"})
            ns0.must_reach("192.168.10.2")

        with test.step("Send tagged frames with DSCP 46, verify PCP and DSCP are untouched"):
            def send_plain():
                for n, (pcp, _) in enumerate(PCP_CASES):
                    set_pcp(pcp)
                    ns0.runsh(f"ping -c1 -w2 -Q {46 << 2} -e {300 + n} 192.168.10.2")

            # A fabric that encodes the PCP from the priority keeps its
            # table with remarking off, and says so in the rewrite table
            rewr = tgtssh.runsh(f"dcb rewr show dev {td1}").stdout
            encoded = "prio-pcp" in rewr
            print(f"{td1} encodes PCP from priority: {encoded}")

            seen = capture(send_plain)
            for n, (pcp, prio) in enumerate(PCP_CASES):
                assert 300 + n in seen, f"no echo request with PCP {pcp} captured"
                want = (10, prio if encoded else pcp, 46)
                assert seen[300 + n] == want, f"PCP {pcp}: got {seen[300 + n]}, expected {want}"

    test.succeed()
