#!/usr/bin/env python3

"""Bridge VLAN

Verify VLAN filtering bridge, with a VLAN trunk to a neighboring device,
which in turn untags one VLAN outisde a non-VLAN filtering bridge.

.Logical network setup
image::bridge-vlan.svg[]

"""
import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        dut1 = env.attach("dut1", "mgmt")
        dut2 = env.attach("dut2", "mgmt")

    with test.step("Configure DUTs"):
        dut1.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "bridge": {
                            "vlans": {
                                "vlan": [
                                    {
                                        "vid": 10,
                                        "untagged": [dut1["data"]],
                                        "tagged":   [dut1["link"], "br0"]
                                    }
                                ]
                            }
                        }
                    }, {
                        "name": "vlan10",
                        "type": "infix-if-type:vlan",
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
                    }, {
                        "name": dut1["data"],
                        "infix-interfaces:bridge-port": {
                            "pvid": 10,
                            "bridge": "br0"
                        }
                    }, {
                        "name": dut1["link"],
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
                        "ipv4": {
                            "address": [
                                {
                                    "ip": "10.0.0.3",
                                    "prefix-length": 24,
                                }
                            ]
                        }
                    }, {
                        "name": dut2["link"],
                    }, {
                        "name": "e0.10",
                        "type": "infix-if-type:vlan",
                        "vlan": {
                            "lower-layer-if": dut2["link"],
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
