#!/usr/bin/env python3
"""LAN-WAN Firewall with Masquerading

Typical home/office router scenario where the DUT acts as a gateway with
LAN-to-WAN traffic forwarding and masquerading (SNAT).

image::lan-wan.svg[align=center, scaledwidth=50%]

- DUT/Gateway with firewall and NAT
- Test host has two interfaces: a LAN-side and a WAN-side (Internet)
- Test host's LAN interface acts as a client behind the router
- Test host's WAN interface acts as an Internet server/destination
"""

import time
import infamy
from infamy.util import until


with infamy.Test() as test:
    with test.step("Set up topology and attach to gateway"):
        env = infamy.Env()
        gateway = env.attach("gateway", "mgmt")
        _, lan_if = env.ltop.xlate("gateway", "lan")
        _, wan_if = env.ltop.xlate("gateway", "wan")
        _, mgmt_if = env.ltop.xlate("gateway", "mgmt")
        _, host_lan = env.ltop.xlate("host", "lan")  # Host LAN-side interface
        _, host_wan = env.ltop.xlate("host", "wan")  # Host WAN-side interface

        LAN_NET = "192.168.1.0/24"
        LAN_ROUTER_IP = "192.168.1.1"     # Router's LAN interface
        LAN_CLIENT_IP = "192.168.1.100"   # Client on LAN side

        WAN_NET = "203.0.113.0/24"        # RFC 5737 test network
        WAN_ROUTER_IP = "203.0.113.1"     # Router's WAN interface
        WAN_SERVER_IP = "203.0.113.100"   # Server on WAN side

    with test.step("Configure gateway with firewall and SNAT"):
        gateway.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": lan_if,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": LAN_ROUTER_IP,
                                "prefix-length": 24
                            }]
                        }
                    },
                    {
                        "name": wan_if,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": WAN_ROUTER_IP,
                                "prefix-length": 24
                            }]
                        }
                    }
                ]
            }
        })

        gateway.put_config_dict("infix-firewall", {
            "firewall": {
                "default": "wan",
                "logging": "all",
                "zone": [
                    {
                        "name": "lan",
                        "description": "Internal LAN network - trusted",
                        "action": "accept",
                        "interface": [lan_if, mgmt_if],
                        "service": ["ssh", "dhcp", "dns"]
                    }, {
                        "name": "wan",
                        "description": "External WAN interface - untrusted",
                        "action": "drop",
                        "interface": [wan_if]
                    }
                ],
                "policy": [
                    {
                        "name": "lan-to-wan",
                        "description": "Allow LAN to WAN traffic with SNAT",
                        "ingress": ["lan"],
                        "egress": ["wan"],
                        "action": "accept",
                        "masquerade": True
                    }
                ]
            }
        })

        # Wait for configuration to be activated
        infamy.Firewall.wait_for_operational(gateway, {
            "lan": {"action": "accept"},
            "wan": {"action": "drop"}
        })

        # Verify firewall operational state
        data = gateway.get_data("/infix-firewall:firewall")
        fw = data["firewall"]
        zones = {z["name"]: z for z in fw["zone"]}

        # Verify LAN zone
        lan_zone = zones["lan"]
        assert lan_zone["action"] == "accept"
        assert lan_if in lan_zone["interface"]

        # Verify WAN zone
        wan_zone = zones["wan"]
        assert wan_zone["action"] == "drop"
        assert wan_if in wan_zone["interface"]

        # Verify policy
        policies = {p["name"]: p for p in fw["policy"]}
        lan_wan_policy = policies["lan-to-wan"]
        assert lan_wan_policy["ingress"] == ["lan"]
        assert lan_wan_policy["egress"] == ["wan"]
        assert lan_wan_policy["action"] == "accept"
        assert lan_wan_policy["masquerade"] is True

    with infamy.IsolatedMacVlan(host_lan) as lan_client:
        lan_client.addip(LAN_CLIENT_IP)
        lan_client.addroute("0.0.0.0", LAN_ROUTER_IP, prefix_length="0")

        with infamy.IsolatedMacVlan(host_wan) as wan_server:
            wan_server.addip(WAN_SERVER_IP)

            with test.step("Verify LAN access to router"):
                lan_client.must_reach(LAN_ROUTER_IP, timeout=3)

            with test.step("Verify LAN services accessibility"):
                firewall = infamy.Firewall(lan_client, None)
                svc = [
                    (22, "tcp", "ssh"),
                    (53, "udp", "dns"),
                    (67, "udp", "dhcp"),
                ]

                ok, ports = firewall.verify_allowed(LAN_ROUTER_IP, svc)
                if not ok:
                    print(f"   ⚠ Some LAN services are filtered: {', '.join(ports)}")
                    test.fail()

            with test.step("Verify WAN access to router is blocked"):
                wan_server.must_not_reach(WAN_ROUTER_IP, timeout=3)

            with test.step("Verify WAN blocks all well-known ports"):
                firewall = infamy.Firewall(wan_server, None)

                ok, ports, _ = firewall.verify_blocked(WAN_ROUTER_IP)
                if not ok:
                    print(f"   ⚠ Some ports are unexpectedly open from WAN: {', '.join(ports)}")
                    test.fail()

            with test.step("Verify LAN-to-WAN connectivity (outbound)"):
                lan_client.must_reach(WAN_SERVER_IP, timeout=3)

            with test.step("Verify LAN-to-WAN masquerading"):
                firewall = infamy.Firewall(lan_client, wan_server)

                ok, info = firewall.verify_snat(WAN_SERVER_IP, WAN_ROUTER_IP)
                if not ok:
                    print(f"   ⚠ {info}")
                    test.fail()

            with test.step("Verify WAN-to-LAN blocking (inbound)"):
                wan_server.must_not_reach(LAN_CLIENT_IP, timeout=3)

    test.succeed()
