#!/usr/bin/env python3
"""WAN-DMZ-LAN Firewall with Port Forwarding

Multi-zone firewall setup with port forwarding (DNAT) to a DMZ server,
and masquerading (SNAT) of WAN-bound traffic.

image::wan-dmz-lan.svg[align=center, scaledwidth=50%]

- DUT/Gateway with WAN/DMZ/LAN zones and NAT
- Test host's WAN interface acts as external Internet client
- Test host's DMZ interface acts as internal server (HTTP on port 80)
- Test host's LAN interface acts as internal LAN client
"""

import time
import infamy
from infamy.util import until


with infamy.Test() as test:
    with test.step("Set up topology and attach to gateway"):
        env = infamy.Env()
        gateway = env.attach("gateway", "mgmt")
        _, wan_if = env.ltop.xlate("gateway", "wan")
        _, dmz_if = env.ltop.xlate("gateway", "dmz")
        _, lan_if = env.ltop.xlate("gateway", "lan")
        _, mgmt_if = env.ltop.xlate("gateway", "mgmt")
        _, host_wan = env.ltop.xlate("host", "wan")
        _, host_dmz = env.ltop.xlate("host", "dmz")
        _, host_lan = env.ltop.xlate("host", "lan")

        WAN_NET = "203.0.113.0/24"        # RFC 5737 test network
        WAN_ROUTER_IP = "203.0.113.1"     # Gateway WAN interface
        WAN_CLIENT_IP = "203.0.113.100"   # Host WAN interface

        DMZ_NET = "10.0.1.0/24"
        DMZ_ROUTER_IP = "10.0.1.1"        # Gateway DMZ interface
        DMZ_SERVER_IP = "10.0.1.100"      # Host DMZ interface

        LAN_NET = "192.168.1.0/24"
        LAN_ROUTER_IP = "192.168.1.1"     # Gateway LAN interface
        LAN_CLIENT_IP = "192.168.1.100"   # Host LAN interface

    with test.step("Configure gateway with multi-zone firewall and NAT"):
        gateway.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
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
                    },
                    {
                        "name": dmz_if,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": True,
                            "address": [{
                                "ip": DMZ_ROUTER_IP,
                                "prefix-length": 24
                            }]
                        }
                    },
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
                        "name": "wan",
                        "description": "External WAN interface - untrusted",
                        "action": "drop",
                        "interface": [wan_if],
                        "port-forward": [{
                            "lower": 8080,
                            "proto": "tcp",
                            "to": {
                                "addr": DMZ_SERVER_IP,
                                "port": 80
                            }
                        }]
                    },
                    {
                        "name": "dmz",
                        "description": "DMZ network - limited trust",
                        "action": "reject",
                        "network": [DMZ_NET],
                        "service": ["http"]
                    },
                    {
                        "name": "lan",
                        "description": "Internal LAN network - trusted",
                        "action": "accept",
                        "interface": [lan_if, mgmt_if]
                    }
                ],
                "policy": [
                    {
                        "name": "loc-to-wan",
                        "description": "Allow local networks to WAN with SNAT",
                        "ingress": ["lan", "dmz"],
                        "egress": ["wan"],
                        "action": "accept",
                        "masquerade": True
                    }, {
                        "name": "lan-to-dmz",
                        "description": "Allow LAN access to DMZ services",
                        "ingress": ["lan"],
                        "egress": ["dmz"],
                        "action": "accept",
                        "service": ["ssh", "http"]
                    }
                ]
            }
        })

        # Wait for configuration to be activated
        infamy.Firewall.wait_for_operational(gateway, {
            "wan": {"action": "drop"},
            "dmz": {"action": "reject"},
            "lan": {"action": "accept"}
        })

        # Verify firewall operational state
        data = gateway.get_data("/infix-firewall:firewall")
        fw = data["firewall"]
        zones = {z["name"]: z for z in fw["zone"]}

        # Verify WAN zone with port forwarding
        wan_zone = zones["wan"]
        assert wan_zone["action"] == "drop"
        assert wan_if in wan_zone["interface"]
        assert len(wan_zone["port-forward"]) == 1
        pf = next(iter(wan_zone["port-forward"]))
        assert pf["lower"] == 8080
        assert pf["to"]["addr"] == DMZ_SERVER_IP
        assert pf["to"]["port"] == 80

        # Verify DMZ zone
        dmz_zone = zones["dmz"]
        assert dmz_zone["action"] == "reject"
        assert DMZ_NET in dmz_zone["network"]
        assert "http" in dmz_zone["service"]

        # Verify LAN zone
        lan_zone = zones["lan"]
        assert lan_zone["action"] == "accept"
        assert lan_if in lan_zone["interface"]

        # Check policies
        policies = {p["name"]: p for p in fw["policy"]}

        # Verify loc-to-wan policy
        loc_wan_policy = policies["loc-to-wan"]
        assert set(loc_wan_policy["ingress"]) == {"lan", "dmz"}
        assert loc_wan_policy["egress"] == ["wan"]
        assert loc_wan_policy["masquerade"] is True

        # Verify lan-to-dmz policy
        lan_dmz_policy = policies["lan-to-dmz"]
        assert lan_dmz_policy["ingress"] == ["lan"]
        assert lan_dmz_policy["egress"] == ["dmz"]
        assert "ssh" in lan_dmz_policy["service"]
        assert "http" in lan_dmz_policy["service"]

    with infamy.IsolatedMacVlan(host_wan) as wan_client:
        wan_client.addip(WAN_CLIENT_IP)

        with infamy.IsolatedMacVlan(host_dmz) as dmz_server:
            dmz_server.addip(DMZ_SERVER_IP)
            dmz_server.addroute("0.0.0.0", DMZ_ROUTER_IP, prefix_length="0")

            with infamy.IsolatedMacVlan(host_lan) as lan_client:
                lan_client.addip(LAN_CLIENT_IP)
                lan_client.addroute("0.0.0.0", LAN_ROUTER_IP, prefix_length="0")

                with test.step("Verify basic connectivity within zones"):
                    lan_client.must_reach(LAN_ROUTER_IP, timeout=3)
                    dmz_server.must_not_reach(DMZ_ROUTER_IP, timeout=3)

                with test.step("Verify WAN to DMZ port forwarding (DNAT)"):
                    firewall = infamy.Firewall(wan_client, dmz_server)

                    # Test port forwarding: WAN:8080 → DMZ:80
                    ok, info = firewall.verify_dnat(
                        WAN_ROUTER_IP, forward_port=8080, target_port=80)

                    if not ok:
                        print(f"   ⚠ {info}")
                        test.fail()

                with test.step("Verify LAN to DMZ connectivity"):
                    lan_client.must_reach(DMZ_SERVER_IP, timeout=3)
                    firewall = infamy.Firewall(lan_client, None)
                    svc = [
                        (22, "tcp", "ssh"),
                        (80, "tcp", "http"),
                    ]

                    ok, ports = firewall.verify_allowed(DMZ_SERVER_IP, svc)
                    if not ok:
                        print(f"   ⚠ Some DMZ services filtered from LAN: {', '.join(ports)}")
                        test.fail()

                with test.step("Verify DMZ to LAN blocking"):
                    dmz_server.must_not_reach(LAN_CLIENT_IP, timeout=3)

                with test.step("Verify WAN isolation"):
                    firewall = infamy.Firewall(wan_client, None)

                    ok, ports, _ = firewall.verify_blocked(LAN_ROUTER_IP)
                    if not ok:
                        print(f"   ⚠ WAN can access LAN ports: {', '.join(ports)}")
                        test.fail()

                    ok, ports, _ = firewall.verify_blocked(DMZ_ROUTER_IP)
                    if not ok:
                        print(f"   ⚠ WAN can access DMZ ports: {', '.join(ports)}")

                with test.step("Verify LAN to WAN connectivity with SNAT"):
                    firewall = infamy.Firewall(lan_client, wan_client)

                    lan_client.must_reach(WAN_CLIENT_IP, timeout=3)

                    ok, info = firewall.verify_snat(WAN_CLIENT_IP, WAN_ROUTER_IP)
                    if not ok:
                        print(f"   ⚠ LAN to WAN SNAT: {info}")
                        test.fail()

                with test.step("Verify DMZ to WAN connectivity with SNAT"):
                    firewall = infamy.Firewall(dmz_server, wan_client)

                    dmz_server.must_reach(WAN_CLIENT_IP, timeout=3)

                    ok, info = firewall.verify_snat(WAN_CLIENT_IP, WAN_ROUTER_IP)
                    if not ok:
                        print(f"   ⚠ DMZ to WAN SNAT: {info}")
                        test.fail()

                with test.step("Verify zone default actions/services"):
                    firewall_lan = infamy.Firewall(lan_client, None)
                    firewall_dmz = infamy.Firewall(dmz_server, None)
                    firewall_wan = infamy.Firewall(wan_client, None)

                    svc = [
                        (22, "tcp", "ssh"),
                        (53, "udp", "dns"),
                        (67, "udp", "dhcp")
                    ]
                    ok, ports = firewall_lan.verify_allowed(LAN_ROUTER_IP, svc)
                    if not ok:
                        print(f"   ⚠ LAN services not properly accessible: {', '.join(ports)}")

                    svc = [(80, "tcp", "http")]
                    ok, ports = firewall_dmz.verify_allowed(DMZ_ROUTER_IP, svc)
                    if not ok:
                        print(f"   ⚠ DMZ HTTP service not accessible: {', '.join(ports)}")

                    ok, ports, _ = firewall_wan.verify_blocked(WAN_ROUTER_IP)
                    if not ok:
                        print(f"   ⚠ WAN has unexpected open ports: {', '.join(ports)}")

    test.succeed()
