#!/usr/bin/env python3
# ,------------------------------------------------,
# |                 [TARGET]                       |
# |                                                |
# |                                                |
# | target:mgmt   target:data0     targetgt:data1  |
# |              [192.168.0.1]     [10.0.0.1]      |
# '------------------------------------------------'
#         |                  |     |
#         |                  |     |
# ,----------------------------------------------,
# |   host:mgmt    host:data0       host:data1   |
# |             [192.168.0.10]     [10.0.0.10]   |
# |                  (ns0)            (ns1)      |
# |                                              |
# |                  [ HOST ]                    |
# '----------------------------------------------'

"""
Routing basic

Verify routing between interfaces is possible. That enable/disable routing
in configuration has the expected result.
"""
import infamy

def config_target(target, tport0, tport1, enable_fwd):
    target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": tport0,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": enable_fwd,
                            "address": [{
                            "ip": "192.168.0.1",
                            "prefix-length": 24
                            }]
                        }
                    },
                    {
                        "name": tport1,
                        "enabled": True,
                        "ipv4": {
                            "forwarding": enable_fwd,
                            "address": [{
                            "ip": "10.0.0.1",
                            "prefix-length": 24
                            }]
                        }
                    }
                ]
            }
        })

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, tport0 = env.ltop.xlate("target", "data0")
        _, tport1 = env.ltop.xlate("target", "data1")

        _, hport0 = env.ltop.xlate("host", "data0")
        _, hport1 = env.ltop.xlate("host", "data1")

    with infamy.IsolatedMacVlan(hport0) as ns0, \
         infamy.IsolatedMacVlan(hport1) as ns1 :

        with test.step("Setup host"):
            ns0.addip("192.168.0.10")
            ns0.addroute("default", "192.168.0.1")

            ns1.addip("10.0.0.10")
            ns1.addroute("default", "10.0.0.1")

        with test.step("Enable forwarding on target:data0 and target:data1"):
            config_target(target, tport0, tport1, True)

        with test.step("Verify ping from host:data0 to 10.0.0.10"):
            ns0.must_reach("10.0.0.10")

        with test.step("Verify ping from host:data1 to 192.168.0.10"):
            ns1.must_reach("192.168.0.10")

        with test.step("Disable forwarding on target:data0 and target:data1"):
            config_target(target, tport0, tport1, False)

        with test.step("Verify ping does not work host:data0->10.0.0.10 and host:data1->192.168.0.10"):
            infamy.parallel(lambda: ns0.must_not_reach("10.0.0.10"),
                            lambda: ns1.must_not_reach("192.168.0.10"))

    test.succeed()
