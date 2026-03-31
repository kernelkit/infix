#!/usr/bin/env python3
"""Basic Firewall for End Devices

Firewall configuration suitable for end devices on untrusted networks.

image::basic.svg[align=center, scaledwidth=50%]

- Single zone configuration, "public-untrusted-net", with `action=drop`
- Allowed services: SSH (port 22), DHCPv6-client, mySSH (custom, port 222)
- All other ports (HTTP, HTTPS, Telnet, etc.) blocked
- Check that unused interfaces are automatically assigned to default zone
"""

import time
import infamy
from infamy.util import until


with infamy.Test() as test:
    with test.step("Set up topology and attach to target"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, data_if = env.ltop.xlate("target", "data")
        _, mgmt_if = env.ltop.xlate("target", "mgmt")
        _, unused_if = env.ltop.xlate("target", "unused")
        _, host_data = env.ltop.xlate("host", "data")
        TARGET_IP = "192.168.1.1"
        HOST_IP = "192.168.1.42"

    with test.step("Configure basic end-device firewall"):
        target.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": data_if,
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": TARGET_IP,
                                "prefix-length": 24
                            }]
                        }
                    }
                ]
            }
        },
        "infix-firewall": {
            "firewall": {
                "default": "public-untrusted-net",
                "logging": "all",
                "service": [{
                    "name": "mySSH",
                    "port": [{
                        "lower": 222,
                        "proto": "tcp"
                    }]
                }, {
                    "name": "http",
                    "port": [{
                        "lower": 8080,
                        "proto": "tcp"
                    }]
                }],
                "zone": [{
                    "name": "mgmt",
                    "description": "Management network - for test automation",
                    "action": "accept",
                    "interface": [mgmt_if],
                    "service": ["ssh", "netconf", "restconf"]
                }, {
                    # 20-char name, exceeds old iptables-derived 17-char limit
                    # Verifies we allow long names with nftables, issue #1389
                    "name": "public-untrusted-net",
                    "description": "Public untrusted network",
                    "action": "drop",
                    "interface": [data_if],
                    "service": ["ssh", "dhcpv6-client", "mySSH", "http"]
                }]
            }
        }})

        # Wait for configuration to be activated
        infamy.Firewall.wait_for_operational(target, {
            "public-untrusted-net": {
                "action": "drop",
                "service": ["ssh", "dhcpv6-client", "mySSH", "http"]
            },
            "mgmt": {
                "action": "accept",
                "service": ["ssh", "netconf", "restconf"]
            }
        })

        # Verify firewall operational state
        data = target.get_data("/infix-firewall:firewall")
        fw = data["firewall"]

        assert fw["default"] == "public-untrusted-net"

        zones = {zone["name"]: zone for zone in fw["zone"]}
        assert "public-untrusted-net" in zones, "public-untrusted-net zone not found"
        public_zone = zones["public-untrusted-net"]
        assert public_zone["action"] == "drop"
        assert data_if in public_zone["interface"]
        assert "ssh" in public_zone["service"]
        assert "dhcpv6-client" in public_zone["service"]
        assert "mySSH" in public_zone["service"]
        assert "http" in public_zone["service"]

    with test.step("Verify unused interface assigned to default zone"):
        data = target.get_data("/infix-firewall:firewall")
        fw = data["firewall"]

        assert fw["default"] == "public-untrusted-net", "Default zone should be 'public-untrusted-net'"

        zones = {zone["name"]: zone for zone in fw["zone"]}
        public_zone = zones["public-untrusted-net"]

        assert unused_if in public_zone["interface"], \
            f"Unused interface {unused_if} should be in default zone 'public-untrusted-net', got interfaces: {public_zone['interface']}"

    with infamy.IsolatedMacVlan(host_data) as ns:
        ns.addip(HOST_IP)

        with test.step("Verify ICMP is dropped"):
            ns.must_not_reach(TARGET_IP, timeout=2)

        with test.step("Verify ICMPv6 is dropped"):
            ns.must_not_reach("fe80::1%iface", timeout=2)

        with test.step("Verify SSH service is allowed"):
            scanner = infamy.PortScanner(ns)
            ssh_result = scanner.scan_port(TARGET_IP, 22, timeout=2)
            assert ssh_result["status"] in ["open", "closed"], \
                f"SSH port should be allowed, got: {ssh_result['status']}"

        with test.step("Verify custom mySSH service is allowed"):
            scanner = infamy.PortScanner(ns)
            myssh_result = scanner.scan_port(TARGET_IP, 222, timeout=2)
            assert myssh_result["status"] in ["open", "closed"], \
                f"mySSH port 222 should be allowed, got: {myssh_result['status']}"

        with test.step("Verify HTTP service override (8080 allowed, 80 blocked)"):
            scanner = infamy.PortScanner(ns)

            # Custom HTTP on port 8080 should be allowed
            http_custom_result = scanner.scan_port(TARGET_IP, 8080, timeout=2)
            assert http_custom_result["status"] in ["open", "closed"], \
                f"Custom HTTP port 8080 should be allowed, got: {http_custom_result['status']}"

            # Built-in HTTP on port 80 should be blocked (filtered)
            http_builtin_result = scanner.scan_port(TARGET_IP, 80, timeout=2)
            assert http_builtin_result["status"] == "filtered", \
                f"Built-in HTTP port 80 should be blocked, got: {http_builtin_result['status']}"

        with test.step("Verify other ports are blocked"):
            firewall = infamy.Firewall(ns, None)
            allowed = [22, 222, 8080]

            ok, open_ports, filtered = \
                firewall.verify_blocked(TARGET_IP, exempt=allowed)
            if not ok:
                if open_ports:
                    print(f"Unexpected open ports: {', '.join(open_ports)}")
                if filtered:
                    print(f"Unexpected, filtered ports: {', '.join(filtered)}")
                test.fail()

    test.succeed()
