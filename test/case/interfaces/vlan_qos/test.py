#!/usr/bin/env python3
#  .----------------.
#  |     target     |
#  |                |
#  | d0.10    d1.11 |
#  |    |       |   |
#  '---d0------d1---'
#       |       |
#  .---d0------d1---.
#  |                |
#  |      host      |
#  '----------------'
#
# Inject tagged packets on host/d1, let target route it over to d2
# where the tag priority is snooped, with different combinations of
# pcp<->priority mappings and verify the results.
"""
VLAN Interface Ingress/Egress QoS

Inject different packets from the host and verify that the VLAN priority
is handled correctly for traffic ingressing and egressing a VLAN interface.

The test verifies that ingress PCP priority is mapped correctly to internal
priority ('from pcp' or static '0..7'), and that internal priority is mapped
correctly to PCP on egress ('from internal priority' or static '0..7').

    { "id": 1, "pcp": 0, "ingress": 0,          "egress": 0,               "expect": 0 },
    { "id": 2, "pcp": 0, "ingress": 0,          "egress": 1,               "expect": 1 },
    { "id": 3, "pcp": 0, "ingress": 3,          "egress": 0,               "expect": 0 },
    { "id": 4, "pcp": 0, "ingress": 4,          "egress": "from-priority", "expect": 4 },
    { "id": 5, "pcp": 5, "ingress": "from-pcp", "egress": "from-priority", "expect": 5 },
    { "id": 6, "pcp": 6, "ingress": "from-pcp", "egress": "from-priority", "expect": 6 },


"""
import infamy

# Each line represents an injected ICMP request with `id` (tagged with
# `pcp`), where the target uses `ingress` to map PCP to priority and
# `egress` to map priority to PCP, with the `expect`ed outgoing PCP.
PACKETS = (
    { "id": 1, "pcp": 0, "ingress": 0,          "egress": 0,               "expect": 0 },
    { "id": 2, "pcp": 0, "ingress": 0,          "egress": 1,               "expect": 1 },
    { "id": 3, "pcp": 0, "ingress": 3,          "egress": 0,               "expect": 0 },
    { "id": 4, "pcp": 0, "ingress": 4,          "egress": "from-priority", "expect": 4 },
    { "id": 5, "pcp": 5, "ingress": "from-pcp", "egress": "from-priority", "expect": 5 },
    { "id": 6, "pcp": 6, "ingress": "from-pcp", "egress": "from-priority", "expect": 6 },
)

def verify_priority_mapping(p):
    expect = f"192.168.10.2 > 192.168.11.2: ICMP echo request, id {p['id']}"
    assert expect in packets, f"Missing id={p['id']}"
    expect = f"vlan 11, p {p['expect']}, ethertype IPv4 (0x0800), " + expect
    assert expect in packets, f"PCP mismatch for id={p['id']}"

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, td0 = env.ltop.xlate("target", "data0")
        _, td1 = env.ltop.xlate("target", "data1")

        _, hd0 = env.ltop.xlate("host", "data0")
        _, hd1 = env.ltop.xlate("host", "data1")

    with test.step("Apply initial config without priority mapping"):
        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": td0,
                        "enabled": True,
                    },
                    {
                        "name": "vlan10",
                        "type": "infix-if-type:vlan",
                        "vlan": {
                            "id": 10,
                            "lower-layer-if": td0,
                        },
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "192.168.10.1",
                                "prefix-length": 24
                            }]
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
                        },
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": "192.168.11.1",
                                "prefix-length": 24
                            }]
                        }
                    }
                ]
            }
        })

    with infamy.IsolatedMacVlan(hd0) as ns0, \
         infamy.IsolatedMacVlan(hd1) as ns1 :

        with test.step("Setup host VLANs"):
            for ns, vid in ((ns0, 10), (ns1, 11)):
                ns.runsh(f"""
                set -ex

                ip link set iface up
                ip link add dev vlan{vid} link iface up type vlan id {vid}
                ip addr add 192.168.{vid}.2/24 dev vlan{vid}
                """)
                ns.addroute("default", f"192.168.{vid}.1")

            ns0.must_reach("192.168.11.2")

        pcap = ns1.pcap("vlan 11 and icmp[icmptype] == icmp-echo")
        with pcap:
            for p in PACKETS:
                desc  = f"Inject id={p['id']}, pcp={p['pcp']} "
                desc += f"(ingress-prio={p['ingress']}, egress-pcp={p['egress']})"

                with test.step(desc):
                    ns0.runsh(f"ip link set vlan10 type vlan egress-qos-map 0:{p['pcp']}")

                    target.put_config_dict("ietf-interfaces", {
                        "interfaces": {
                            "interface": [
                                {
                                    "name": "vlan10",
                                    "vlan": {
                                        "ingress-qos": {
                                            "priority": p["ingress"]
                                        }
                                    },
                                },
                                {
                                    "name": "vlan11",
                                    "vlan": {
                                        "egress-qos": {
                                            "pcp": p["egress"]
                                        }
                                    },
                                },
                            ]
                        }
                    })

                    ns0.must_reach("192.168.11.2", id=p["id"])

        packets = pcap.tcpdump("-e")
        print(packets)

        pkt=PACKETS[0]
        with test.step ("Verify that packet with id=1 egressed with pcp=0"):
            verify_priority_mapping(pkt)

        pkt=PACKETS[1]
        with test.step ("Verify that packet with id=2 egressed with pcp=1"):
            verify_priority_mapping(pkt)

        pkt=PACKETS[2]
        with test.step ("Verify that packet with id=3 egressed with pcp=0"):
            verify_priority_mapping(pkt)       

        pkt=PACKETS[3]
        with test.step ("Verify that packet with id=4 egressed with pcp=4"):
            verify_priority_mapping(pkt)
            
        pkt=PACKETS[4]
        with test.step ("Verify that packet with id=5 egressed with pcp=5"):
            verify_priority_mapping(pkt)                  

        pkt=PACKETS[5]
        with test.step ("Verify that packet with id=6 egressed with pcp=6"):
            verify_priority_mapping(pkt)       
            
    test.succeed()
