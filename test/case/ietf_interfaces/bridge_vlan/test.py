#!/usr/bin/env python3

"""
Bridge VLAN

Basic test of VLAN functionality in a bridge, tagged/untagged traffic and a VLAN interface in the bridge.
....
           ¦                              ¦
           ¦       vlan10 IP:10.0.0.2     ¦        br0  IP:10.0.0.3
           ¦       /                      ¦       /
           ¦     br0  <-- VLAN filtering  ¦   link.10
           ¦   u/  \\t                     ¦    /
   PC ------data    link -----------------|-- link
           ¦    dut1                      ¦   dut2
....

"""
import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env  = infamy.Env()
        dut1 = env.attach("dut1", "mgmt")
        dut2 = env.attach("dut2", "mgmt")

        _, dut1_e0 = env.ltop.xlate("dut1", "data")
        _, dut1_e1 = env.ltop.xlate("dut1", "link")
        _, dut2_e0 = env.ltop.xlate("dut2", "link")

    with test.step("Configure DUTs"):
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

    with test.step("Verify ping from host:data to 10.0.0.2 and 10.0.0.3"):
        _, hport = env.ltop.xlate("host", "data")

        with infamy.IsolatedMacVlan(hport) as ns:
            ns.addip("10.0.0.1")
            ns.must_reach("10.0.0.2")
            ns.must_reach("10.0.0.3")

    test.succeed()
