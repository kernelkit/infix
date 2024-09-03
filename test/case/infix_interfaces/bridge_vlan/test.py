#!/usr/bin/env python3
#           ¦                              ¦
#           ¦       vlan10 IP:10.0.0.2     ¦        br0  IP:10.0.0.3
#           ¦       /                      ¦       /
#           ¦     br0  <-- VLAN filtering  ¦     e0.10
#           ¦   u/  \t                     ¦    /
#   PC ------- e0    e1 ---------------------- e0
# PING -->  ¦             dut1             ¦            dut2
#
"""
Bridge VLAN

Basic test of VLAN functionality in a bridge
"""
import infamy

with infamy.Test() as test:
    with test.step("Initialize"):
        env  = infamy.Env()
        dut1 = env.attach("target1", "mgmt")
        dut2 = env.attach("target2", "mgmt")

    with test.step("Topology setup"):
        _, dut1_e0 = env.ltop.xlate("target1", "data")
        _, dut1_e1 = env.ltop.xlate("target1", "target2")
        _, dut2_e0 = env.ltop.xlate("target2", "target1")

        dut1.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                        "bridge": {
                            "vlans": {
                                "pvid": 4094,
                                "vlan": [
                                    {
                                        "vid": 10,
                                        "untagged": [ dut1_e0 ],
                                        "tagged":   [ "br0", dut1_e1 ]
                                    }
                                ]
                            }
                        }
                    },
                    {
                        "name": "vlan10",
                        "type": "infix-if-type:vlan",
                        "enabled": True,
                        "vlan": {
                            "lower-layer-if": "br0",
                            "id": 10,
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
                    {
                        "name": dut1_e0,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "pvid": 10,
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": dut1_e1,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "pvid": 10,
                            "bridge": "br0"
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
                        "ipv4": {
                            "address": [
                                {
                                    "ip": "10.0.0.3",
                                    "prefix-length": 24,
                                }
                            ]
                        }
                    },
                    {
                        "name": dut2_e0,
                        "enabled": True
                    },
                    {
                        "name": "e0.10",
                        "type": "infix-if-type:vlan",
                        "enabled": True,
                        "vlan": {
                            "lower-layer-if": dut2_e0,
                            "id": 10,
                        },
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    }
                ]
            }
        })

    with test.step("Connectivity check"):
        _, hport = env.ltop.xlate("host", "data1")

        with infamy.IsolatedMacVlan(hport) as ns:
            ns.addip("10.0.0.1")
            ns.must_reach("10.0.0.2")
            ns.must_reach("10.0.0.3")

    test.succeed()
