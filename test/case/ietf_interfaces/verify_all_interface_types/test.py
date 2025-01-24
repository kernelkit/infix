#!/usr/bin/env python3
r"""Verify that all interface types can be created

This test verifies that all interface types can be created and also
checks the configuration when applied sequentially. This method takes
slightly longer than sending the entire configuration at once.

....

 lo     br-0                    br-Q.40            br-D         br-X
  |       |                       |                  |            |
  o       o        ethQ.10       br-Q            veth0a.20      ethX.30
                          \     /    \               |            |
                           ethQ       veth0b       veth0a        ethX
                                          `---------'
....

"""

import infamy
import infamy.iface as iface


def verify_interface(target, interface, expected_type):
    assert iface.exist(target, interface), f"Interface <{interface}> does not exist."

    expected_type = f"infix-if-type:{expected_type}"
    actual_type = iface.get_param(target, interface, "type")

    if expected_type == "infix-if-type:etherlike"  and actual_type == "infix-if-type:ethernet":
        return  # Allow 'etherlike' to match 'ethernet'

    assert actual_type == expected_type, f"Assertion failed! expected tpye: {expected_type}, actual type {actual_type}"


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env  = infamy.Env()
        target = env.attach("target", "mgmt")

        _, eth_Q = env.ltop.xlate("target", "ethQ")
        _, eth_X = env.ltop.xlate("target", "ethX")

        eth_Q_10 = f"{eth_Q}.10"
        eth_X_30 = f"{eth_X}.30"

        br_0 = "br-0"
        br_X = "br-X"
        br_D = "br-D"
        br_Q = "br-Q"

        veth_a = "veth0a"
        veth_b = "veth0b"

        veth_a_20 = f"{veth_a}.20"
        br_Q_40 = f"{br_Q}.40"

        loopback = "lo"

    with test.step("Configure an empty bridge br-0"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                    {
                        "name": br_0,
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                    }
                    ]
                }
            }
        })

    with test.step("Configure bridge br-X and associated interfaces"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": br_X,
                            "type": "infix-if-type:bridge",
                            "enabled": True
                        },
                        {
                            "name": eth_X_30,
                            "type": "infix-if-type:vlan",
                            "enabled": True,
                            "vlan": {
                                "lower-layer-if": eth_X,
                                "id": 30
                            },
                            "infix-interfaces:bridge-port": {
                                "bridge": br_X
                            }
                        }
                    ]
                }
            }
    })

    with test.step("Configure VETH pair"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": veth_a,
                            "type": "infix-if-type:veth",
                            "enabled": True,
                            "infix-interfaces:veth": {
                                "peer": veth_b
                            }
                        },
                        {
                            "name": veth_b,
                            "type": "infix-if-type:veth",
                            "enabled": True,
                            "infix-interfaces:veth": {
                                "peer": veth_a
                            }
                        }
                    ]
                }
            }
        })

    with test.step("Configure bridge br-D and associated interfaces"):
        target.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": br_D,
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                        "ietf-ip:ipv4": {
                            "address": [
                                { "ip": "192.168.20.1", "prefix-length": 24 },
                                { "ip": "10.0.0.1", "prefix-length": 8 },
                            ],
                        },
                        "ietf-ip:ipv6": {
                            "address": [
                                { "ip": "2001:db8::1", "prefix-length": 64 },
                            ],
                        },
                    },
                    {
                        "name": veth_a_20,
                        "type": "infix-if-type:vlan",
                        "enabled": True,
                        "vlan": {
                            "lower-layer-if": "veth0a",
                            "id": 20
                        },
                        "infix-interfaces:bridge-port": {
                            "bridge": br_D
                        }
                    }
                ]
            }
        }
    })

    with test.step("Configure br-Q and associated interfaces"):
        target.put_config_dicts({
            "ietf-interfaces": {
            "interfaces": {
                "interface": [
                    {
                        "name": br_Q,
                        "type": "infix-if-type:bridge",
                        "enabled": True,
                        "infix-interfaces:bridge": {
                            "vlans": {
                                "vlan": [
                                    { "vid": 20, "untagged": [br_Q], "tagged": [eth_Q, veth_b] },
                                    { "vid": 30, "untagged": [br_Q], "tagged": [eth_Q, veth_b] },
                                    { "vid": 40, "untagged": [], "tagged": [br_Q, eth_Q, veth_b] },
                                ],
                            }
                        },
                        "infix-interfaces:bridge-port": {
                            "pvid": 10,
                        }
                    },
                    {
                        "name": eth_Q,
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": br_Q
                        }
                    },
                    {
                        "name": veth_b,
                        "type": "infix-if-type:veth",
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": br_Q
                        }
                    },
                    {
                        "name": eth_Q_10,
                        "type": "infix-if-type:vlan",
                        "enabled": True,
                        "vlan": {
                            "lower-layer-if": eth_Q,
                            "id": 10
                        }
                    },
                    {
                        "name": br_Q_40,
                        "type": "infix-if-type:vlan",
                        "enabled": True,
                        "vlan": {
                            "lower-layer-if": br_Q,
                            "id": 40
                        }
                    }
                ]
            }
        }
    })

    with test.step("Configure GRE Tunnels"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": "gre-v4",
                            "type": "infix-if-type:gre",

                            "infix-interfaces:gre": {
                                "local": "192.168.20.1",
                                "remote": "192.168.20.2",
                            }
                        },
                        {
                            "name": "gre-v6",
                            "type": "infix-if-type:gre",
                            "ietf-ip:ipv4": {
                                "address": [
                                    { "ip": "192.168.50.2", "prefix-length": 16 },
                                ]
                        },
                            "infix-interfaces:gre": {
                                "local": "2001:db8::1",
                                "remote": "2001:db8::2",
                            }
                        },
                        {
                            "name": "gretap-v4",
                            "type": "infix-if-type:gretap",
                            "infix-interfaces:gre": {
                                "local": "192.168.20.1",
                                "remote": "192.168.20.2",
                            }
                        },
                        {
                            "name": "gretap-v6",
                            "type": "infix-if-type:gretap",
                            "infix-interfaces:gre": {
                                "local": "2001:db8::1",
                                "remote": "2001:db8::2",
                            }
                        },
                    ]
                }
            }
        })

    with test.step("Configure VxLAN Tunnels"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                    {
                        "name": "vxlan-v4",
                        "type": "infix-if-type:vxlan",
                        "ietf-ip:ipv4": {
                            "address": [
                                { "ip": "192.168.30.2", "prefix-length": 24 },
                            ]
                        },
                        "infix-interfaces:vxlan": {
                            "local": "192.168.20.100",
                            "remote": "192.168.20.200",
                            "vni": "4"
                        }
                    },
                    {
                        "name": "vxlan-v6",
                        "type": "infix-if-type:vxlan",
                        "ietf-ip:ipv4": {
                            "address": [
                                { "ip": "192.168.40.2", "prefix-length": 24 },
                            ]
                        },
                        "infix-interfaces:vxlan": {
                            "local": "2001:db8::100",
                            "remote": "2001:db8::200",
                            "vni": "6"
                        }
                    }
                ]
            }
        }
    })

    with test.step("Verify interface 'lo' is of type loopback"):
        verify_interface(target, "lo", "loopback")

    with test.step("Verify interfaces 'ethX' and 'ethQ' are of type 'ethernet' (or etherlike if running Qemu)"):
         verify_interface(target, eth_X, "etherlike")
         verify_interface(target, eth_Q, "etherlike")

    with test.step("Verify interfaces 'br-0', 'br-X', 'br-D' and 'br-Q' are of type 'bridge'"):
        verify_interface(target, "br-0", "bridge")
        verify_interface(target, "br-X", "bridge")
        verify_interface(target, "br-Q", "bridge")
        verify_interface(target, "br-D", "bridge")

    with test.step("Verify interfaces 'veth0a' and 'veth0b' are of type 'veth'"):
        verify_interface(target, "veth0a", "veth")
        verify_interface(target, "veth0b", "veth")

    with test.step("Verify interfaces 'veth0a.20', 'ethQ.10', 'ethX.30', 'ethQ.10' and 'br-Q.40' are of type 'vlan'"):
        verify_interface(target, "veth0a.20", "vlan")
        verify_interface(target, f"{eth_X}.30", "vlan")
        verify_interface(target, f"{eth_Q}.10", "vlan")
        verify_interface(target, "br-Q.40", "vlan")

    with test.step("Verify GRE interfaces 'gre-v4', 'gre-v6', 'gretap-v4' and 'gretap-v6'"):
        verify_interface(target, "gre-v4", "gre")
        verify_interface(target, "gre-v6", "gre")
        verify_interface(target, "gretap-v4", "gretap")
        verify_interface(target, "gretap-v6", "gretap")

    with test.step("Verify VxLAN interfaces 'vxlan-v4' and 'vxlan-v6'"):
        verify_interface(target, "vxlan-v4", "vxlan")
        verify_interface(target, "vxlan-v6", "vxlan")
    test.succeed()
