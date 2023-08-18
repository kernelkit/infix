#!/usr/bin/env python3
# ,------------------------------------------------,       
# |                 [TARGET]                       | 
# |                            br0                 |  
# |                           /   \                | 
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

import infamy

def config_target(target, tport0, tport1, enable_fwd):
    target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": tport0,
                        "type": "iana-if-type:ethernetCsmacd",
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
                        "type": "iana-if-type:ethernetCsmacd",
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
        env = infamy.Env(infamy.std_topology("1x3"))
        target = env.attach("target", "mgmt")

    with test.step("Configure target physical ports [enable routing]"):
        _, tport0 = env.ltop.xlate("target", "data0")
        _, tport1 = env.ltop.xlate("target", "data1")

        config_target(target, tport0, tport1, True)

    with test.step("Test: Ip routing enabled"):
        _, hport0 = env.ltop.xlate("host", "data0")
        _, hport1 = env.ltop.xlate("host", "data1")

        with infamy.IsolatedMacVlan(hport0) as ns0, \
             infamy.IsolatedMacVlan(hport1) as ns1 :
            
            ns0.addip("192.168.0.10")
            ns0.addroute("default", "192.168.0.1")
                
            ns1.addip("10.0.0.10")
            ns1.addroute("default", "10.0.0.1")
            
            ns0.must_reach("10.0.0.10")
            ns1.must_reach("192.168.0.10")

    with test.step("Configure target physical ports [disable routing]"):

        config_target(target, tport0, tport1, False)

    with test.step("Test: Ip routing disabled"):

        with infamy.IsolatedMacVlan(hport0) as ns0, \
             infamy.IsolatedMacVlan(hport1) as ns1 :
            
            ns0.addip("192.168.0.10")
            ns0.addroute("default", "192.168.0.1")
                
            ns1.addip("10.0.0.10")
            ns1.addroute("default", "10.0.0.1")

            ns0.must_not_reach("10.0.0.10")
            ns1.must_not_reach("192.168.0.10")

    test.succeed()
