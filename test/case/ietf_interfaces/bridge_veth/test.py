#!/usr/bin/env python3
"""
Bridge with a physical port and a veth

This tests the possibility to add software added interfaces, in this case
VETH and bridge it with a physical interface

....

PING -->      br0
             /   \\
 PC- target:data  veth0a -- veth0b
     10.0.0.1               10.0.0.2

....


"""
import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Configure bridged eth port and veth pair with IP 10.0.0.2"):
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
                        "name": tport,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
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
                        "ipv4": {
                            "address": [
                                {
                                    "ip": "10.0.0.2",
                                    "prefix-length": 24,
                                }
                            ]
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
