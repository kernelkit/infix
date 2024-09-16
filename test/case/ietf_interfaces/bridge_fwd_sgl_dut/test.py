#!/usr/bin/env python3
# ,-----------------------------------------,
# |                                         |
# |                          br0            |
# |                         /   \           |
# | target:mgmt    tgt:data0     tgt:data1  |
# '-----------------------------------------'
#         |                |     |
#         |                |     |
# ,------------------------------------------,
# |   host:mgmt   host:data0     host:data1  |
# |               [10.0.0.1]     [10.0.0.2]  |
# |                  (ns0)         (ns1)     |
# |                                          |
# |                 [ HOST ]                 |
# '------------------------------------------'
"""
Bridge forwarding single DUTs

Tests forwarding through a DUT with two bridged interfaces
"""
import infamy

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Configure a bridge with dual physical port"):
        _, tport0 = env.ltop.xlate("target", "data0")
        _, tport1 = env.ltop.xlate("target", "data1")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                    },
                    {
                        "name": tport0,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": tport1,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    }
                ]
            }
        })

    with test.step("Ping host:data1 [10.0.0.2] from host:data0 [10.0.0.1]"):
        _, hport0 = env.ltop.xlate("host", "data0")
        _, hport1 = env.ltop.xlate("host", "data1")

        with infamy.IsolatedMacVlan(hport0) as ns0, \
             infamy.IsolatedMacVlan(hport1) as ns1 :

            ns1.addip("10.0.0.2")
            ns0.addip("10.0.0.1")

            ns0.must_reach("10.0.0.2")

    test.succeed()
