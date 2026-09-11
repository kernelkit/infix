#!/usr/bin/env python3
"""
QoS DSCP Classification and Remarking End to End

Send IP packets with different DSCP values into a routed port that trusts
DSCP with the ietf preset, and route them out over a VLAN interface whose
egress PCP is derived from the internal priority.  The PCP on the wire
then reveals the priority the classifier assigned:

    DSCP  0 (CS0)  -> priority 0
    DSCP  8 (CS1)  -> priority 1
    DSCP 18 (AF21) -> priority 2
    DSCP 26 (AF31) -> priority 3
    DSCP 34 (AF41) -> priority 4
    DSCP 46 (EF)   -> priority 5
    DSCP 48 (CS6)  -> priority 6
    DSCP 56 (CS7)  -> priority 7
    DSCP  4        -> priority 0, not in the preset, port default

Then enable DSCP remarking on the egress port and repeat: every packet
must leave with the class selector of its priority, CS0 to CS7, e.g. EF
in, CS5 out.

Works on any port: classification and remarking run in the switch fabric
where the driver supports them and in the kernel otherwise, and routed
traffic passes the kernel in both cases.
"""
import re
import infamy

# DSCP to expected priority per the ietf preset, with a default fallback
CASES = ((0, 0), (8, 1), (18, 2), (26, 3), (34, 4), (46, 5), (48, 6), (56, 7), (4, 0))

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, td0 = env.ltop.xlate("target", "data1")
        _, td1 = env.ltop.xlate("target", "data2")
        _, hd0 = env.ltop.xlate("host", "data1")
        _, hd1 = env.ltop.xlate("host", "data2")

    with test.step("Configure routed ingress port trusting DSCP and VLAN egress from priority"):
        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": td0,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{"ip": "192.168.10.1", "prefix-length": 24}]
                        },
                        "infix-interfaces:qos": {
                            "ingress": {
                                "trust": "dscp",
                                "default-priority": 0,
                                "dscp-map": {"preset": "ietf"},
                            }
                        }
                    },
                    {
                        "name": td1,
                        "enabled": True,
                    },
                    {
                        "name": "vlan11",
                        "type": "infix-if-type:vlan",
                        "vlan": {
                            "id": 11,
                            "lower-layer-if": td1,
                            "egress-qos": {"pcp": "from-priority"},
                        },
                        "ipv4": {
                            "forwarding": True,
                            "address": [{"ip": "192.168.11.1", "prefix-length": 24}]
                        }
                    }
                ]
            }
        }})

    with infamy.IsolatedMacVlan(hd0) as ns0, \
         infamy.IsolatedMacVlan(hd1) as ns1:

        with test.step("Set up host namespaces on both sides"):
            ns0.runsh("""
            set -ex
            ip link set iface up
            ip addr add 192.168.10.2/24 dev iface
            """)
            ns0.addroute("default", "192.168.10.1")

            ns1.runsh("""
            set -ex
            ip link set iface up
            ip link add dev vlan11 link iface up type vlan id 11
            ip addr add 192.168.11.2/24 dev vlan11
            """)
            ns1.addroute("default", "192.168.11.1")

            ns0.must_reach("192.168.11.2")

        def capture(what):
            """Ping once per case with the case index as ICMP id, return {id: (pcp, dscp)}"""
            pcap = ns1.pcap("vlan 11 and icmp[icmptype] == icmp-echo")
            with pcap:
                for n, (dscp, prio) in enumerate(CASES):
                    with test.step(f"Send ICMP echo with DSCP {dscp}, {what} {prio}"):
                        ns0.runsh(f"ping -c1 -w2 -Q {dscp << 2} -e {100 + n} 192.168.11.2")
            packets = pcap.tcpdump("-e -v")
            print(packets)

            seen = {}
            for m in re.finditer(r"vlan 11, p (\d+), .*?\(tos 0x([0-9a-f]+).*?"
                                 r"ICMP echo request, id (\d+)", packets, re.S):
                seen[int(m.group(3))] = (int(m.group(1)), int(m.group(2), 16) >> 2)
            return seen

        seen = capture("expect PCP")
        with test.step("Verify the PCP of each echo request matches its DSCP class"):
            for n, (dscp, prio) in enumerate(CASES):
                assert 100 + n in seen, f"no echo request with DSCP {dscp} captured"
                assert seen[100 + n] == (prio, dscp), f"DSCP {dscp}: got {seen[100 + n]}"

        with test.step("Enable DSCP remarking from priority on the egress port"):
            target.put_config_dicts({"ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": td1,
                        "infix-interfaces:qos": {
                            "egress": {"remark": {"dscp": "from-priority"}}
                        }
                    }]
                }
            }})

            # Applying the change may reprogram the port; wait for the path
            ns0.must_reach("192.168.11.2")

        seen = capture("expect CS")
        with test.step("Verify each echo request leaves with the class selector of its priority"):
            for n, (dscp, prio) in enumerate(CASES):
                assert 100 + n in seen, f"no echo request with DSCP {dscp} captured"
                assert seen[100 + n] == (prio, prio << 3), f"DSCP {dscp}: got {seen[100 + n]}"

    test.succeed()
