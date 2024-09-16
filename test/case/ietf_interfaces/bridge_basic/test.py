#!/usr/bin/env python3
#
# PING -->     br0 10.0.0.2
#             /
#   PC ---- e0
#
"""
Bridge basic

Test basic connectivity to a bridge
"""
import infamy

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Configure single bridge with a single physical port, bridge @ IP 10.0.0.2"):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
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
                        "name": tport,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                ]
            }
        })

    with test.step("Ping bridge 10.0.0.2 from host:data with IP 10.0.0.1"):
        _, hport = env.ltop.xlate("host", "data")

        with infamy.IsolatedMacVlan(hport) as ns:
            ns.addip("10.0.0.1")
            ns.must_reach("10.0.0.2")

    test.succeed()
