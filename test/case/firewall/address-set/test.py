#!/usr/bin/env python3
"""Firewall Address-Sets with Dynamic Entries

Verifies firewall address-sets used as zone sources, in a setup where
all traffic from the data network is dropped by default and end devices
are granted access per-IP at runtime.

image::topology.svg[align=center, scaledwidth=50%]

- The default zone "public" drops all traffic on the data interface
- The "trusted" zone accepts traffic from members of the "allowed"
  address-set: one static entry from the configuration, and dynamic
  entries managed at runtime with the add/remove/flush actions
- Dynamic entries must survive unrelated firewall configuration
  changes, but are not saved to the configuration
- Static entries cannot be removed with the remove action
- Entries in timeout sets ("greylist") expire on their own
"""

import infamy
from infamy.util import until


def get_address_set(target, name):
    """Fetch operational state for a named address-set"""
    data = target.get_data("/infix-firewall:firewall")
    sets = data["firewall"].get("address-set", [])
    return next((s for s in sets if s["name"] == name), None)


def current_entries(target, name):
    """Return {entry: dynamic} for the live contents of an address-set"""
    aset = get_address_set(target, name)
    if not aset:
        return {}
    return {c["entry"]: c["dynamic"] for c in aset.get("current", [])}


def must_fail(fn, *args):
    """Assert that an action call raises an error"""
    try:
        fn(*args)
    except Exception:
        return
    raise AssertionError(f"{args} unexpectedly succeeded")


with infamy.Test() as test:
    ALLOWED = "/infix-firewall:firewall/address-set[name='allowed']"
    GREYLIST = "/infix-firewall:firewall/address-set[name='greylist']"

    with test.step("Set up topology and attach to target"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, data_if = env.ltop.xlate("target", "data")
        _, mgmt_if = env.ltop.xlate("target", "mgmt")
        _, host_data = env.ltop.xlate("host", "data")
        TARGET_IP = "192.168.1.1"
        HOST_IP = "192.168.1.42"
        STATIC_IP = "192.168.1.40"
        EXTRA_IP = "192.168.1.50"

    with test.step("Configure firewall with address-set as zone source"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": data_if,
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": TARGET_IP,
                                "prefix-length": 24
                            }]
                        }
                    }]
                }
            },
            "infix-firewall": {
                "firewall": {
                    "default": "public",
                    "logging": "all",
                    "address-set": [{
                        "name": "allowed",
                        "description": "End devices allowed to access the target",
                        "entry": [STATIC_IP]
                    }, {
                        "name": "greylist",
                        "description": "Entries expire on their own",
                        "timeout": 10
                    }],
                    "zone": [{
                        "name": "mgmt",
                        "description": "Management network - for test automation",
                        "action": "accept",
                        "interface": [mgmt_if],
                        "service": ["ssh", "netconf", "restconf"]
                    }, {
                        "name": "public",
                        "description": "Untrusted data network, drop everything",
                        "action": "drop",
                        "interface": [data_if]
                    }, {
                        "name": "trusted",
                        "description": "Allowed end devices",
                        "action": "accept",
                        "address-set": ["allowed"]
                    }]
                }
            }
        })

        infamy.Firewall.wait_for_operational(target, {
            "public": {"action": "drop"},
            "trusted": {"action": "accept"},
            "mgmt": {"action": "accept"}
        })

        aset = get_address_set(target, "allowed")
        assert aset, "Address-set 'allowed' not found in operational"
        assert STATIC_IP in aset.get("entry", []), \
            f"Static entry {STATIC_IP} missing from configuration"

    with infamy.IsolatedMacVlan(host_data) as ns:
        ns.addip(HOST_IP)

        with test.step("Verify host is blocked by default"):
            ns.must_not_reach(TARGET_IP, timeout=5)

        with test.step("Add dynamic entry for host, verify it is allowed"):
            target.call_action(f"{ALLOWED}/add", {"entry": HOST_IP})
            ns.must_reach(TARGET_IP, timeout=10)

        with test.step("Verify operational state distinguishes static/dynamic"):
            def entries_settled():
                current = current_entries(target, "allowed")
                return current.get(STATIC_IP) is False and \
                    current.get(HOST_IP) is True

            until(entries_settled, attempts=10)

        with test.step("Verify dynamic entry survives configuration change"):
            target.put_config_dicts({
                "infix-firewall": {
                    "firewall": {
                        "logging": "unicast"
                    }
                }
            })

            ns.must_reach(TARGET_IP, timeout=30)
            current = current_entries(target, "allowed")
            assert current.get(HOST_IP) is True, \
                f"Dynamic entry {HOST_IP} lost in firewall reload"

        with test.step("Verify static entry cannot be removed with action"):
            must_fail(target.call_action, f"{ALLOWED}/remove",
                      {"entry": STATIC_IP})

        with test.step("Remove dynamic entry, verify host is blocked"):
            target.call_action(f"{ALLOWED}/remove", {"entry": HOST_IP})
            ns.must_not_reach(TARGET_IP, timeout=5)

        with test.step("Flush dynamic entries, static remain"):
            target.call_action(f"{ALLOWED}/add", {"entry": HOST_IP})
            target.call_action(f"{ALLOWED}/add", {"entry": EXTRA_IP})
            target.call_action(f"{ALLOWED}/flush")

            def only_static_left():
                current = current_entries(target, "allowed")
                return list(current.keys()) == [STATIC_IP]

            until(only_static_left, attempts=10)
            ns.must_not_reach(TARGET_IP, timeout=5)

    with test.step("Verify entries in timeout set expire on their own"):
        target.call_action(f"{GREYLIST}/add", {"entry": "10.0.0.1"})

        aset = get_address_set(target, "greylist")
        assert aset, "Address-set 'greylist' not found in operational"
        current = {c["entry"]: c for c in aset.get("current", [])}
        assert "10.0.0.1" in current, "Entry missing from timeout set"
        assert current["10.0.0.1"].get("expires") is not None, \
            "Entry in timeout set has no expiry"

        must_fail(target.call_action, f"{GREYLIST}/remove",
                  {"entry": "10.0.0.1"})

        def entry_expired():
            return "10.0.0.1" not in current_entries(target, "greylist")

        until(entry_expired, attempts=20)

    test.succeed()
