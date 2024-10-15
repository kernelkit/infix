#!/usr/bin/env python3
r"""
Bridge forwarding single DUTs

Tests forwarding through a DUT with two bridged interfaces on one DUT.

....

,------------------------------------------,
|                                          |
|                          br0             |
|                         /   \            |
| target:mgmt   target:data1  target:data2 |
'------------------------------------------'
        |                |     |
        |                |     |
,------------------------------------------,
|   host:mgmt   host:data1     host:data2  |
|               [10.0.0.1]     [10.0.0.2]  |
|                  (ns0)         (ns1)     |
|                                          |
|                 [ HOST ]                 |
'------------------------------------------'

....

"""
import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Configure a bridge with dual physical port"):
        _, tport1 = env.ltop.xlate("target", "data1")
        _, tport2 = env.ltop.xlate("target", "data2")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                    },
                    {
                        "name": tport1,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": tport2,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    }
                ]
            }
        })

    with test.step("Verify ping from host:data0 to 10.0.0.1"):
        _, hport1 = env.ltop.xlate("host", "data1")
        _, hport2 = env.ltop.xlate("host", "data2")

        with infamy.IsolatedMacVlan(hport1) as ns1, \
             infamy.IsolatedMacVlan(hport2) as ns2 :

            ns2.addip("10.0.0.2")
            ns1.addip("10.0.0.1")

            ns1.must_reach("10.0.0.2")

    test.succeed()
