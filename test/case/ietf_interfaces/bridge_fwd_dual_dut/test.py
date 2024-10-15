#!/usr/bin/env python3
r"""
Bridge forwarding dual DUTs

Ping through two bridges on two different DUTs.

....

   ,-------------------------------------,       ,-------------------------------------,
   |                          dut1:link  |       | dut2:link                           |
   |                      br0  ----------|-------|---------  br0                       |
   |                     /               |       |          /   \                      |
   |dut1:mgmt       dut1:data1           |       | dut2:data1    dut2:data2  dut2:mgmt |
   '-------------------------------------'       '-------------------------------------'
       |                |                                  |     |                 |
       |                |                                  |     |                 |
,-----------------------------------------------------------------------------------------,
|  host:mgmt1    host:data11                      host:data21    host:data22   host:mgmt2 |
|                [10.0.0.2]                       [10.0.0.3]     [10.0.0.4]               |
|                  (ns11)                           (ns20)         (ns21)                 |
|                                                                                         |
|                                        [ HOST ]                                         |
'-----------------------------------------------------------------------------------------'

....

"""

import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        dut1 = env.attach("dut1", "mgmt")
        dut2 = env.attach("dut2", "mgmt")

    with test.step("Configure a bridge with triple physical port"):
        _, tport11 = env.ltop.xlate("dut1", "data1")
        _, tport1_link = env.ltop.xlate("dut1", "link")
        _, tport21 = env.ltop.xlate("dut2", "data1")
        _, tport22 = env.ltop.xlate("dut2", "data2")
        _, tport2_link = env.ltop.xlate("dut2", "link")

        dut1.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                        "bridge": {
                            "vlans": {
                                "vlan": [
                                    {
                                        "vid": 10,
                                        "untagged": [ tport11 ],
                                        "tagged":   [ "br0", tport1_link ]
                                    }
                                ]
                            }
                        }
                    },
                    {
                        "name": tport11,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "pvid": 10,
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": tport1_link,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0",
                        }
                    }
                ]
            }
        })

        dut2.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                        "bridge": {
                            "vlans": {
                                "vlan": [
                                    {
                                        "vid": 10,
                                        "untagged": [ tport21, tport22 ],
                                        "tagged":   [ "br0", tport2_link ]
                                    }
                                ]
                            }
                        }
                    },
                    {
                        "name": tport21,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "pvid": 10,
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": tport22,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "pvid": 10,
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": tport2_link,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0",
                        }
                    }
                ]
            }
        })

    with test.step("Verify ping 10.0.0.3 and 10.0.0.4 from host:data11"):

        _, hport11 = env.ltop.xlate("host", "data11")
        _, hport21 = env.ltop.xlate("host", "data21")
        _, hport22 = env.ltop.xlate("host", "data22")

        with infamy.IsolatedMacVlan(hport11) as ns11, \
             infamy.IsolatedMacVlan(hport21) as ns21, \
             infamy.IsolatedMacVlan(hport22) as ns22:

            ns11.addip("10.0.0.2")
            ns21.addip("10.0.0.3")
            ns22.addip("10.0.0.4")

            ns11.must_reach("10.0.0.3")
            ns11.must_reach("10.0.0.4")

    test.succeed()
