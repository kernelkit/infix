#!/usr/bin/env python3
"""
Routing basic

Verify that the ietf-ip forwarding setting controls whether IPv4
and IPv6 traffic is routed between interfaces.  When forwarding is
enabled, hosts on separate subnets can reach each other through
the device.  When forwarding is disabled, that connectivity is
expected to be lost.
"""
import infamy

SUBNETS = [
    {"ipv4": {"gw": "192.168.0.1",   "host": "192.168.0.10",   "prefix": 24},
     "ipv6": {"gw": "2001:db8:0::1", "host": "2001:db8:0::10", "prefix": 64}},
    {"ipv4": {"gw": "10.0.0.1",      "host": "10.0.0.10",      "prefix": 24},
     "ipv6": {"gw": "2001:db8:1::1", "host": "2001:db8:1::10", "prefix": 64}},
]

def iface_cfg(port, subnet, enable_fwd):
    """Build interface config with both IPv4 and IPv6."""
    cfg = {"name": port, "enabled": True}
    for family in ("ipv4", "ipv6"):
        af = subnet[family]
        cfg[family] = {
            "forwarding": enable_fwd,
            "address": [{"ip": af["gw"], "prefix-length": af["prefix"]}],
        }
    return cfg

def config_target(target, tport0, tport1, enable_fwd):
    """Configure forwarding and addresses for both address families."""
    target.put_config_dict("ietf-interfaces", {
        "interfaces": {
            "interface": [
                iface_cfg(tport0, SUBNETS[0], enable_fwd),
                iface_cfg(tport1, SUBNETS[1], enable_fwd),
            ]
        }
    })

def setup_host(ns, subnet):
    """Add IPv4 and IPv6 addresses and default routes."""
    v4 = subnet["ipv4"]
    ns.addip(v4["host"])
    ns.addroute("default", v4["gw"])

    v6 = subnet["ipv6"]
    ns.addip(v6["host"], prefix_length=v6["prefix"], proto="ipv6")
    ns.addroute("default", v6["gw"], proto="ipv6")

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, tport0 = env.ltop.xlate("target", "data1")
        _, tport1 = env.ltop.xlate("target", "data2")

        _, hport0 = env.ltop.xlate("host", "data1")
        _, hport1 = env.ltop.xlate("host", "data2")

    with infamy.IsolatedMacVlan(hport0) as ns0, \
         infamy.IsolatedMacVlan(hport1) as ns1:

        with test.step("Set up host addresses and default routes"):
            setup_host(ns0, SUBNETS[0])
            setup_host(ns1, SUBNETS[1])

        with test.step("Enable forwarding on target:data1 and target:data2"):
            config_target(target, tport0, tport1, True)

        with test.step("Verify cross-subnet IPv4 connectivity"):
            ns0.must_reach(SUBNETS[1]["ipv4"]["host"])
            ns1.must_reach(SUBNETS[0]["ipv4"]["host"])

        with test.step("Verify cross-subnet IPv6 connectivity"):
            ns0.must_reach(SUBNETS[1]["ipv6"]["host"])
            ns1.must_reach(SUBNETS[0]["ipv6"]["host"])

        with test.step("Disable forwarding on target:data1 and target:data2"):
            config_target(target, tport0, tport1, False)

        with test.step("Verify cross-subnet connectivity is lost"):
            infamy.parallel(
                lambda: ns0.must_not_reach(SUBNETS[1]["ipv4"]["host"]),
                lambda: ns1.must_not_reach(SUBNETS[0]["ipv4"]["host"]),
                lambda: ns0.must_not_reach(SUBNETS[1]["ipv6"]["host"]),
                lambda: ns1.must_not_reach(SUBNETS[0]["ipv6"]["host"]))

    test.succeed()
