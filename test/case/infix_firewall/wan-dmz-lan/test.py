#!/usr/bin/env python3
"""
WAN-DMZ-LAN Firewall with Port Forwarding

Verifies a comprehensive multi-zone firewall setup with port forwarding (DNAT)
and masquerading (SNAT).

Architecture:
- Target device = Gateway with WAN/DMZ/LAN zones and NAT
- Test host has four interfaces: WAN (Internet), DMZ (server zone),
  LAN (internal), mgmt
- Host WAN interface acts as external Internet client
- Host DMZ interface acts as internal server (HTTP on port 80)
- Host LAN interface acts as internal LAN client

The test verifies:
- WAN zone with action=drop for external interface
- DMZ zone with limited services (HTTP only)
- LAN zone with action=accept for internal network
- Port forwarding: WAN:8080 → DMZ:80 (DNAT)
- Policy loc-to-wan: DMZ+LAN → WAN with masquerading (SNAT)
- Policy lan-to-dmz: LAN → DMZ for SSH and HTTP access
- Proper zone isolation and access control
- End-to-end DNAT and SNAT functionality

This validates complex firewall scenarios with both ingress NAT (DNAT)
and egress NAT (SNAT) plus comprehensive multi-zone policies.
"""
from time import sleep
import infamy
from infamy.util import until

# Some helper classes return extra debug info which this switch unlocks
DEBUG = False


def debug(msg):
    """Debug messages not relevant for regular test output"""
    if DEBUG:
        print(msg)


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
                        "port-forward": [
                            {
                                "lower": 8080,
                                "proto": "tcp",
                                "to": {
                                    "addr": DMZ_SERVER_IP,
                                    "port": 80
                                }
                            }
                        ]
                    },
                    {
                        "name": "dmz",
                        "description": "DMZ network - limited trust",
                        "action": "drop",
                        "interface": [dmz_if],
                        "service": ["http"]
                    },
                    {
                        "name": "lan",
                        "description": "Internal LAN network - trusted",
                        "action": "accept",
                        "interface": [lan_if, mgmt_if],
                        "service": ["ssh", "dhcp", "dns"]
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
                    },
                    {
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

        debug("Waiting for gateway configuration to take effect...")
        sleep(2)

        # Verify firewall configuration
        config = gateway.get_config_dict("/infix-firewall:firewall")
        fw = config["firewall"]

        assert fw["default"] == "wan"
        assert len(fw["zone"]) == 3
        assert len(fw["policy"]) == 2

        # Check zones
        zones = {z["name"]: z for z in fw["zone"]}

        # Verify WAN zone with port forwarding
        wan_zone = zones["wan"]
        assert wan_zone["action"] == "drop"
        assert wan_if in wan_zone["interface"]
        assert len(wan_zone["port-forward"]) == 1
        # Access port-forward by iterating over the keyed list
        pf = next(iter(wan_zone["port-forward"]))
        assert pf["lower"] == 8080
        assert pf["to"]["addr"] == DMZ_SERVER_IP
        assert pf["to"]["port"] == 80

        # Verify DMZ zone
        dmz_zone = zones["dmz"]
        assert dmz_zone["action"] == "drop"
        assert dmz_if in dmz_zone["interface"]
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
                    dnat_working, details = firewall.verify_dnat(
                        WAN_ROUTER_IP, forward_port=8080, target_port=80)

                    if dnat_working:
                        debug(f"   ✓ {details}")
                    else:
                        print(f"   ⚠ {details}")
                        test.fail("DNAT port forwarding verification failed")

                with test.step("Verify LAN to DMZ connectivity"):
                    lan_client.must_reach(DMZ_SERVER_IP, timeout=3)
                    firewall = infamy.Firewall(lan_client, None)
                    svc = [
                        (22, "tcp", "ssh"),
                        (80, "tcp", "http"),
                    ]

                    ok, filtered = firewall.verify_allowed(DMZ_SERVER_IP, svc)
                    if ok:
                        debug("   ✓ LAN can access DMZ services (SSH, HTTP)")
                    else:
                        print(f"   ⚠ Some DMZ services filtered from LAN: {', '.join(filtered)}")

                with test.step("Verify DMZ to LAN blocking"):
                    # DMZ should NOT be able to reach LAN (no policy exists)
                    dmz_server.must_not_reach(LAN_CLIENT_IP, timeout=3)

                with test.step("Verify WAN isolation"):
                    # WAN should NOT be able to reach LAN or DMZ directly
                    firewall = infamy.Firewall(wan_client, None)

                    # Test LAN isolation from WAN
                    ok, open_ports, _ = firewall.verify_blocked(LAN_ROUTER_IP)
                    if ok:
                        debug("   ✓ LAN is isolated from WAN")
                    else:
                        print(f"   ⚠ WAN can access LAN ports: {', '.join(open_ports)}")
                        test.fail("LAN not properly isolated from WAN")

                    # Test DMZ isolation from WAN (except port forwarding)
                    ok, open_ports, _ = firewall.verify_blocked(DMZ_ROUTER_IP)
                    if ok:
                        debug("   ✓ DMZ is isolated from WAN")
                    else:
                        print(f"   ⚠ WAN can access DMZ ports: {', '.join(open_ports)}")

                with test.step("Verify LAN to WAN connectivity with SNAT"):
                    # Test outbound connectivity
                    lan_client.must_reach(WAN_CLIENT_IP, timeout=3)

                    # Verify SNAT masquerading
                    firewall = infamy.Firewall(lan_client, wan_client)
                    ok, info = firewall.verify_snat(WAN_CLIENT_IP, WAN_ROUTER_IP)
                    if ok:
                        debug(f"   ✓ LAN to WAN SNAT: {info}")
                    else:
                        print(f"   ⚠ LAN to WAN SNAT: {info}")
                        test.fail("LAN to WAN SNAT verification failed")

                with test.step("Verify DMZ to WAN connectivity with SNAT"):
                    # Test outbound connectivity
                    dmz_server.must_reach(WAN_CLIENT_IP, timeout=3)

                    # Verify SNAT masquerading
                    firewall = infamy.Firewall(dmz_server, wan_client)
                    ok, info = firewall.verify_snat(WAN_CLIENT_IP, WAN_ROUTER_IP)
                    if ok:
                        debug(f"   ✓ DMZ to WAN SNAT: {info}")
                    else:
                        print(f"   ⚠ DMZ to WAN SNAT: {info}")
                        test.fail("DMZ to WAN SNAT verification failed")

                with test.step("Verify comprehensive zone policies"):
                    # Test that each zone has appropriate service accessibility
                    firewall_lan = infamy.Firewall(lan_client, None)
                    firewall_dmz = infamy.Firewall(dmz_server, None)
                    firewall_wan = infamy.Firewall(wan_client, None)

                    # LAN zone should allow configured services
                    svc = [
                        (22, "tcp", "ssh"),
                        (53, "udp", "dns"),
                        (67, "udp", "dhcp")
                    ]
                    ok, filtered = firewall_lan.verify_allowed(LAN_ROUTER_IP, svc)
                    if not ok:
                        print(f"   ⚠ LAN services not properly accessible: {', '.join(filtered)}")

                    # DMZ zone should allow only HTTP
                    svc = [(80, "tcp", "http")]
                    ok, filtered = firewall_dmz.verify_allowed(DMZ_ROUTER_IP, svc)
                    if not ok:
                        print(f"   ⚠ DMZ HTTP service not accessible: {', '.join(filtered)}")

                    # WAN zone should block all standard services
                    ok, open_ports, _ = firewall_wan.verify_blocked(WAN_ROUTER_IP)
                    if not ok:
                        print(f"   ⚠ WAN has unexpected open ports: {', '.join(open_ports)}")

    test.succeed()
