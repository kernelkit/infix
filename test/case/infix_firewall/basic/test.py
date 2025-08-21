#!/usr/bin/env python3
"""
Basic firewall test for end devices

Test a simple restrictive firewall configuration suitable for end devices
like laptops or phones on untrusted networks.

The test verifies:
- Single zone configuration similar to firewalld's default "public" zone
- Zone "public" with action=drop for data interface
- Zone "mgmt" with action=accept for management interface (NETCONF/RESTCONF)
- Allowed services: SSH (port 22), DHCPv6-client
- Blocked: All other ports (HTTP, HTTPS, Telnet, etc.)

Port scanning tests validate that:
- SSH access is allowed (for management)
- All non-essential services are blocked/filtered
- Firewall properly filters unwanted traffic

Uses the infamy.PortScanner class with netcat to test port accessibility:
- Open: Port accessible (service running)
- Closed: Port not filtered but no service listening
- Filtered: Port blocked by firewall (timeout/drop)

This provides suitable protection for end-device scenarios while
maintaining management access for testing.
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
    with test.step("Set up topology and attach to target"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, data_if = env.ltop.xlate("target", "data")
        _, mgmt_if = env.ltop.xlate("target", "mgmt")
        _, host_data = env.ltop.xlate("host", "data")

        # Get target IP for scanning
        TARGET_IP = "192.168.1.1"
        HOST_IP = "192.168.1.42"

    with test.step("Configure basic end-device firewall"):
        target.put_config_dict("ietf-interfaces", {
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
        })

        # Configure firewall with management and public zones
        target.put_config_dict("infix-firewall", {
            "firewall": {
                "default": "public",
                "logging": "all",
                "zone": [
                    {
                        "name": "mgmt",
                        "description": "Management network - allow NETCONF/RESTCONF",
                        "action": "accept",
                        "interface": [mgmt_if],
                        "service": ["ssh", "netconf", "restconf"]
                    },
                    {
                        "name": "public",
                        "description": "Public untrusted network - end device protection",
                        "action": "drop",
                        "interface": [data_if],
                        "service": ["ssh", "dhcpv6-client"]
                    }
                ]
            }
        })

        debug("Waiting for configuration to take ...")
        sleep(1)

        # Verify firewall configuration
        config = target.get_config_dict("/infix-firewall:firewall")
        fw = config["firewall"]

        assert fw["default"] == "public"
        assert len(fw["zone"]) == 2

        # Check zones
        zones = {z["name"]: z for z in fw["zone"]}

        # Verify management zone
        mgmt_zone = zones["mgmt"]
        assert mgmt_zone["action"] == "accept"
        assert mgmt_if in mgmt_zone["interface"]
        assert "ssh" in mgmt_zone["service"]
        assert "netconf" in mgmt_zone["service"]

        # Verify public zone
        public_zone = zones["public"]
        assert public_zone["action"] == "drop"
        assert data_if in public_zone["interface"]
        assert "ssh" in public_zone["service"]
        assert "dhcpv6-client" in public_zone["service"]

    with infamy.IsolatedMacVlan(host_data) as ns:
        ns.addip(HOST_IP)

        with test.step("Verify ICMP is dropped"):
            ns.must_not_reach(TARGET_IP, timeout=2)

        with test.step("Verify ICMPv6 is dropped"):
            ns.must_not_reach6("fe80::1%iface", timeout=2)

        with test.step("Verify SSH port (22) is allowed"):
            scanner = infamy.PortScanner(ns)
            ssh_result = scanner.scan_port(TARGET_IP, 22, timeout=2)
            assert ssh_result["status"] in ["open", "closed"], \
                f"SSH port should be allowed, got: {ssh_result['status']}"

        with test.step("Verify well-known ports are blocked except SSH"):
            firewall = infamy.Firewall(ns, None)
            allowed = [22]  # Only SSH should be allowed

            ok, open_ports, filtered_ports = \
                firewall.verify_blocked(TARGET_IP, exempt=allowed)

            for port in open_ports:
                print(f"   ⚠ Unexpectedly open: {port}")
            for port in filtered_ports:
                print(f"   ⚠ SSH blocked when it should be allowed: {port}")

            if not ok:
                if open_ports:
                    print(f"Found unexpected open ports: {', '.join(open_ports)}")
                    test.fail()
                if filtered_ports:
                    print(f"SSH port blocked when it should be allowed: {', '.join(filtered_ports)}")
                    test.fail()
            else:
                debug("   ✓ Firewall policy working correctly - only SSH allowed")

    test.succeed()
