#!/usr/bin/env python3
"""IPv6 LAN-WAN Firewall

IPv6 version of the typical home/office router scenario where the DUT acts as
a gateway with LAN-to-WAN traffic forwarding and IPv6 prefix delegation.

image::lan-wan.svg[align=center, scaledwidth=50%]

- DUT/Gateway with IPv6 firewall and forwarding
- Test host has two interfaces: a LAN-side and a WAN-side (Internet)
- Test host's LAN interface acts as an IPv6 client behind the router
- Test host's WAN interface acts as an IPv6 Internet server/destination
- Demonstrates IPv6 policy-based forwarding between zones
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

        LAN_NET = "fd01:db8:1::/64"
        LAN_ROUTER_IP = "fd01:db8:1::1"     # Router's LAN interface
        LAN_CLIENT_IP = "fd01:db8:1::100"   # Client on LAN side

        WAN_NET = "2001:db8:2::/64"         # RFC 3849 documentation prefix
        WAN_ROUTER_IP = "2001:db8:2::1"     # Router's WAN interface
        WAN_SERVER_IP = "2001:db8:2::100"   # Server on WAN side

    with test.step("Configure gateway with firewall and forwarding"):
        gateway.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": lan_if,
                        "enabled": True,
                        "ipv6": {
                            "enabled": True,
                            "forwarding": True,
                            "address": [{
                                "ip": LAN_ROUTER_IP,
                                "prefix-length": 64
                            }]
                        }
                    }, {
                        "name": wan_if,
                        "enabled": True,
                        "ipv6": {
                            "enabled": True,
                            "forwarding": True,
                            "address": [{
                                "ip": WAN_ROUTER_IP,
                                "prefix-length": 64
                            }]
                        }
                    }]
                }
            },
            "infix-firewall": {
                "firewall": {
                    "default": "wan",
                    "logging": "all",
                    "zone": [{
                        "name": "lan",
                        "description": "Internal LAN network - trusted",
                        "action": "accept",
                        "interface": [lan_if, mgmt_if],
                        "service": ["ssh", "dhcpv6", "dns"]
                    }, {
                        "name": "wan",
                        "description": "External WAN interface - untrusted",
                        "action": "drop",
                        "interface": [wan_if]
                    }],
                    "policy": [{
                        "name": "lan-to-wan",
                        "description": "Allow LAN to WAN traffic",
                        "ingress": ["lan"],
                        "egress": ["wan"],
                        "action": "accept"
                    }]
                }
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

        # Verify policy exists
        policies = {p["name"]: p for p in fw.get("policy", [])}
        assert "lan-to-wan" in policies
        policy = policies["lan-to-wan"]
        assert "lan" in policy["ingress"]
        assert "wan" in policy["egress"]
        assert policy["action"] == "accept"

    with infamy.IsolatedMacVlan(host_lan) as lan_ns:
        lan_ns.addip(LAN_CLIENT_IP, prefix_length=64, proto="ipv6")
        lan_ns.addroute("default", LAN_ROUTER_IP, proto="ipv6")

        with infamy.IsolatedMacVlan(host_wan) as wan_ns:
            wan_ns.addip(WAN_SERVER_IP, prefix_length=64, proto="ipv6")
            wan_ns.addroute("default", WAN_ROUTER_IP, proto="ipv6")

            with test.step("Test connectivity to gateway"):
                lan_ns.must_reach(LAN_ROUTER_IP, timeout=5)
                wan_ns.must_not_reach(WAN_ROUTER_IP, timeout=5)

            with test.step("Test LAN-to-WAN forwarding"):
                lan_ns.must_reach(WAN_SERVER_IP, timeout=10)

            with test.step("Test WAN-to-LAN blocking"):
                wan_ns.must_not_reach(LAN_CLIENT_IP, timeout=5)

            with test.step("Verify LAN services accessibility"):
                firewall_lan = infamy.Firewall(lan_ns, None)
                svc = [
                    (22, "tcp", "ssh"),
                    (53, "tcp", "dns")
                ]

                ok, ports = firewall_lan.verify_allowed(LAN_ROUTER_IP, svc)
                if not ok:
                    print(f"   ⚠ Some LAN services are filtered: {', '.join(ports)}")
                    test.fail()

    test.succeed()
