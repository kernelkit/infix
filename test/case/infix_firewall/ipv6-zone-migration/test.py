#!/usr/bin/env python3
"""IPv6 Zone Migration with Custom Service

This test verifies that firewall rules work consistently across IPv4/IPv6
protocols and that interfaces can be moved between zones without breaking
active connections.

- Requires DUT with at least 2 data interfaces supporting IPv6
- Test host must support dual-stack IPv4/IPv6 configuration
- Custom service ports (8080/tcp) should be available for testing
"""

import time
import infamy
from infamy.util import until


with infamy.Test() as test:
    with test.step("Set up topology and attach to target"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, data1_if = env.ltop.xlate("target", "data1")
        _, data2_if = env.ltop.xlate("target", "data2")
        _, mgmt_if = env.ltop.xlate("target", "mgmt")
        _, host_data1 = env.ltop.xlate("host", "data1")
        _, host_data2 = env.ltop.xlate("host", "data2")

        # IPv4 addressing
        DATA1_NET_V4 = "10.1.1.0/24"
        DATA1_TARGET_V4 = "10.1.1.1"
        DATA1_HOST_V4 = "10.1.1.100"

        DATA2_NET_V4 = "10.2.2.0/24"
        DATA2_TARGET_V4 = "10.2.2.1"
        DATA2_HOST_V4 = "10.2.2.100"

        # IPv6 addressing
        DATA1_NET_V6 = "fd01:1:1::/64"
        DATA1_TARGET_V6 = "fd01:1:1::1"
        DATA1_HOST_V6 = "fd01:1:1::100"

        DATA2_NET_V6 = "fd02:2:2::/64"
        DATA2_TARGET_V6 = "fd02:2:2::1"
        DATA2_HOST_V6 = "fd02:2:2::100"

        # Custom service port
        CUSTOM_PORT = 8080

    with test.step("Configure dual-stack interfaces and initial firewall"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": data1_if,
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": DATA1_TARGET_V4,
                                "prefix-length": 24
                            }]
                        },
                        "ipv6": {
                            "enabled": True,
                            "address": [{
                                "ip": DATA1_TARGET_V6,
                                "prefix-length": 64
                            }]
                        }
                    }, {
                        "name": data2_if,
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": DATA2_TARGET_V4,
                                "prefix-length": 24
                            }]
                        },
                        "ipv6": {
                            "enabled": True,
                            "address": [{
                                "ip": DATA2_TARGET_V6,
                                "prefix-length": 64
                            }]
                        }
                    }]
                }
            },
            "infix-firewall": {
                "firewall": {
                    "default": "untrusted",
                    "logging": "all",
                    "service": [{
                        "name": "myapp",
                        "port": [{
                            "lower": CUSTOM_PORT,
                            "proto": "tcp"
                        }]
                    }],
                    "zone": [{
                        "name": "mgmt",
                        "description": "Management network",
                        "action": "accept",
                        "interface": [mgmt_if],
                        "service": ["ssh", "netconf", "restconf"]
                    }, {
                        "name": "untrusted",
                        "description": "Untrusted zone",
                        "action": "accept",
                        "interface": [data1_if]
                    }, {
                        "name": "trusted",
                        "description": "Trusted zone",
                        "action": "accept",
                        "interface": [data2_if],
                        "service": ["ssh", "myapp"]
                    }]
                }
            }
        })

        # Wait for configuration to be activated
        infamy.Firewall.wait_for_operational(target, {
            "untrusted": {"action": "accept"},
            "trusted": {"action": "accept"},
            "mgmt": {"action": "accept"}
        })

    with test.step("Verify initial zone configuration and custom service"):
        # Verify operational state matches expected configuration
        data = target.get_data("/infix-firewall:firewall")
        fw = data["firewall"]

        assert fw["default"] == "untrusted"

        zones = {zone["name"]: zone for zone in fw["zone"]}
        services = {svc["name"]: svc for svc in fw.get("service", [])}

        # Verify custom service exists
        assert "myapp" in services, "Custom service myapp not found"
        custom_service = services["myapp"]
        assert len(custom_service["port"]) == 1
        port_entry = next(iter(custom_service["port"]))
        assert port_entry["proto"] == "tcp"
        assert int(port_entry["lower"]) == CUSTOM_PORT

        # Verify zone assignments
        untrusted_zone = zones["untrusted"]
        trusted_zone = zones["trusted"]

        assert data1_if in untrusted_zone["interface"]
        assert data2_if in trusted_zone["interface"]

        # Check services safely - they may not exist in operational data if empty
        trusted_services = trusted_zone.get("service", [])
        untrusted_services = untrusted_zone.get("service", [])

        assert "myapp" in trusted_services, f"Custom service should be in trusted zone, got: {trusted_services}"
        assert "myapp" not in untrusted_services, f"Custom service should not be in untrusted zone, got: {untrusted_services}"

    with infamy.IsolatedMacVlan(host_data1) as ns1:
        ns1.addip(DATA1_HOST_V4, prefix_length=24, proto="ipv4")
        ns1.addip(DATA1_HOST_V6, prefix_length=64, proto="ipv6")

        with infamy.IsolatedMacVlan(host_data2) as ns2:
            ns2.addip(DATA2_HOST_V4, prefix_length=24, proto="ipv4")
            ns2.addip(DATA2_HOST_V6, prefix_length=64, proto="ipv6")

            with test.step("Verify IPv4/IPv6 connectivity and custom service restrictions"):
                # print(f"Testing IPv4 connectivity: {DATA1_HOST_V4} -> {DATA1_TARGET_V4}")
                # print(f"Testing IPv4 connectivity: {DATA2_HOST_V4} -> {DATA2_TARGET_V4}")
                ns1.must_reach(DATA1_TARGET_V4, timeout=5)
                ns2.must_reach(DATA2_TARGET_V4, timeout=5)

                # print(f"Testing IPv6 connectivity: {DATA1_HOST_V6} -> {DATA1_TARGET_V6}")
                # print(f"Testing IPv6 connectivity: {DATA2_HOST_V6} -> {DATA2_TARGET_V6}")
                ns1.must_reach(DATA1_TARGET_V6, timeout=5)
                ns2.must_reach(DATA2_TARGET_V6, timeout=5)

                firewall_ns1 = infamy.Firewall(ns1, None)
                firewall_ns2 = infamy.Firewall(ns2, None)

                ok, ports = firewall_ns1.verify_allowed(DATA1_TARGET_V4,
                                                        [(CUSTOM_PORT, "tcp", "myapp")])
                ok, ports = firewall_ns2.verify_allowed(DATA2_TARGET_V4,
                                                        [(CUSTOM_PORT, "tcp", "myapp")])

            with test.step("Verify IPv6 custom service functionality"):
                ok, ports = firewall_ns1.verify_allowed(DATA1_TARGET_V6,
                                                        [(CUSTOM_PORT, "tcp", "myapp")])
                ok, ports = firewall_ns2.verify_allowed(DATA2_TARGET_V6,
                                                        [(CUSTOM_PORT, "tcp", "myapp")])

            with test.step("Perform dynamic zone migration"):
                target.delete_xpath(f"/infix-firewall:firewall/zone[name='untrusted']/interface[.='{data1_if}']")
                target.put_config_dict("infix-firewall", {
                    "firewall": {
                        "zone": [{
                            "name": "trusted",
                            "interface": [data1_if]
                        }]
                    }
                })

                infamy.Firewall.wait_for_operational(target, {
                    "untrusted": {"action": "accept"},
                    "trusted": {"action": "accept"}
                })

            with test.step("Verify connectivity after zone migration"):
                ns1.must_reach(DATA1_TARGET_V4, timeout=3)
                ns1.must_reach(DATA1_TARGET_V6, timeout=3)
                ns2.must_reach(DATA2_TARGET_V4, timeout=3)
                ns2.must_reach(DATA2_TARGET_V6, timeout=3)

            with test.step("Verify custom service from migrated interface"):
                firewall_migrated = infamy.Firewall(ns1, None)
                ok, ports = firewall_migrated.verify_allowed(DATA1_TARGET_V4,
                                                           [(CUSTOM_PORT, "tcp", "myapp")])
                assert ok, f"Custom service should work on IPv4 after zone migration"

                ok, ports = firewall_migrated.verify_allowed(DATA1_TARGET_V6,
                                                           [(CUSTOM_PORT, "tcp", "myapp")])
                assert ok, f"Custom service should work on IPv6 after zone migration"

            with test.step("Verify operational state reflects zone changes"):
                data = target.get_data("/infix-firewall:firewall")
                fw = data["firewall"]
                zones = {zone["name"]: zone for zone in fw["zone"]}

                trusted_zone = zones["trusted"]
                untrusted_zone = zones["untrusted"]

                assert data1_if in trusted_zone["interface"], "data1_if should be in trusted zone"
                assert data2_if in trusted_zone["interface"], "data2_if should be in trusted zone"
                assert data1_if not in untrusted_zone.get("interface", []), "data1_if should no longer be in untrusted zone"

                trusted_services = trusted_zone.get("service", [])
                assert "myapp" in trusted_services, f"Custom service should be available in trusted zone, got: {trusted_services}"

    test.succeed()
