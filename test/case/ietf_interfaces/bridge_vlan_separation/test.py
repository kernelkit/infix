#!/usr/bin/env python3
r"""
Bridge VLAN separation

Test that two VLANs are correctly separated in the bridge

....
    ,-----------------------------------,   ,----------------------------------,
    |                         dut1:link |   | dut2:link                        |
    |                      br0  --------|---|---------  br0                    |
    |                     / \           |   |          / \                     |
    |dut1:mgmt  dut1:data1   dut1:data2 |   | dut2:data1  dut2:data2 dut2:mgmt |
    '-----------------------------------'   '----------------------------------'
        |             |        |                  |            |         |
        |             |        |                  |            |         |
 ,------------------------------------------------------------------------------,
 |  host:mgmt0 host:data10 host:data11    host:data20  host:data21  host:mgmt1  |
 |             [10.0.0.1]   [10.0.0.2]     [10.0.0.3]     [10.0.0.4]            |
 |               (ns10)       (ns11)         (ns20)         (ns21)              |
 |                                                                              |
 |                                  [ HOST ]                                    |
 '------------------------------------------------------------------------------'

....
"""
import infamy
import subprocess

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        dut1 = env.attach("dut1", "mgmt")
        dut2 = env.attach("dut2", "mgmt")

    with test.step("Configure DUTs"):
        _, tport10 = env.ltop.xlate("dut1", "data1")
        _, tport11 = env.ltop.xlate("dut1", "data2")
        _, tport12 = env.ltop.xlate("dut1", "link")
        _, tport20 = env.ltop.xlate("dut2", "data1")
        _, tport21 = env.ltop.xlate("dut2", "data2")
        _, tport22 = env.ltop.xlate("dut2", "link")

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
                                        "untagged": [ tport10 ],
                                        "tagged":   [ "br0", tport12 ]
                                    },
                                    {
                                        "vid": 20,
                                        "untagged": [ tport11 ],
                                        "tagged":   [ "br0", tport12 ]
                                    }
                                ]
                            }
                        }
                    },
                    {
                        "name": tport10,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "pvid": 10,
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": tport11,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "pvid": 20,
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": tport12,
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
                                        "untagged": [ tport20 ],
                                        "tagged":   [ "br0", tport22 ]
                                    },
                                    {
                                        "vid": 20,
                                        "untagged": [ tport21 ],
                                        "tagged":   [ "br0", tport22 ]
                                    }
                                ]
                            }
                        }
                    },
                    {
                        "name": tport20,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "pvid": 10,
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": tport21,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "pvid": 20,
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": tport22,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0",
                        }
                    }
                ]
            }
        })


    _, hport10 = env.ltop.xlate("host", "data10")
    _, hport11 = env.ltop.xlate("host", "data11")
    _, hport20 = env.ltop.xlate("host", "data20")
    _, hport21 = env.ltop.xlate("host", "data21")

    with infamy.IsolatedMacVlan(hport10) as ns10, \
         infamy.IsolatedMacVlan(hport11) as ns11, \
         infamy.IsolatedMacVlan(hport20) as ns20, \
         infamy.IsolatedMacVlan(hport21) as ns21:

        ns10.addip("10.0.0.1")
        ns11.addip("10.0.0.2")
        ns20.addip("10.0.0.3")
        ns21.addip("10.0.0.4")

        with test.step("Verify ping 10.0.0.3 from host:data10"):
            ns10.must_reach("10.0.0.3")

        with test.step("Verify ping 10.0.0.4 from host:data11"):
            ns11.must_reach("10.0.0.4")

        with test.step("Verify ping not possible host:data10->10.0.0.4, host:data11->10.0.0.3, host:data10->10.0.0.2, host:data11->10.0.0.1"):
            infamy.parallel(lambda: ns10.must_not_reach("10.0.0.4"),
                            lambda: ns11.must_not_reach("10.0.0.3"),
                            lambda: ns10.must_not_reach("10.0.0.2"),
                            lambda: ns11.must_not_reach("10.0.0.1"))

        with test.step("Verify MAC broadcast isolation within VLANs"):
            # Clear ARP entries/queued packets
            ns10.runsh("ip neigh flush all")
            ns11.runsh("ip neigh flush all")
            ns20.runsh("ip neigh flush all")
            ns21.runsh("ip neigh flush all")

            # Sending IP subnet broadcast, resulting in MAC broadcast
            with test.step("Send ping to 10.0.0.255 from host:data10"):
                process=ns10.popen("ping -b -c 5 -i 0.5 10.0.0.255".split(), stderr=subprocess.DEVNULL, stdout=subprocess.DEVNULL)

            with test.step("Verify broadcast is received on host:data20"):
                ns20.must_receive("ip dst 10.0.0.255")

                with test.step("Verify broadcast is NOT received on host:data11 and host:data21"):
                    infamy.parallel(
                        lambda: ns11.must_not_receive("ip dst 10.0.0.255"),
                        lambda: ns21.must_not_receive("ip dst 10.0.0.255")
                    )

    test.succeed()
