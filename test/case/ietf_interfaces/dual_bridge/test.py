#!/usr/bin/env python3
#

"""
Dual bridges on one device

Verify that it is possible to ping through a bridge to another bridge via VETH interfaces.

....
 PING -->     br0             br1 10.0.0.2
             /   \\              /
PC - target:data  veth0a - veth0b
....
"""
import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Configure two bridges linked and a veth pair"):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                    },
                    {
                        "name": "br1",
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                        "ipv4": {
                            "address": [
                                {
                                    "ip": "10.0.0.2",
                                    "prefix-length": 24,
                                }
                            ]
                        }
                    },
                    {
                        "name": "veth0a",
                        "type": "infix-if-type:veth",
                        "enabled": True,
                        "infix-interfaces:veth": {
                            "peer": "veth0b"
                        },
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": "veth0b",
                        "type": "infix-if-type:veth",
                        "enabled": True,
                        "infix-interfaces:veth": {
                            "peer": "veth0a"
                        },
                        "infix-interfaces:bridge-port": {
                            "bridge": "br1"
                        }
                    },
                    {
                        "name": tport,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                ]
            }
        })

    with test.step("Verify ping from host:data to 10.0.0.2"):
        _, hport = env.ltop.xlate("host", "data")

        with infamy.IsolatedMacVlan(hport) as ns:
            ns.addip("10.0.0.1")
            ns.must_reach("10.0.0.2")

    test.succeed()
