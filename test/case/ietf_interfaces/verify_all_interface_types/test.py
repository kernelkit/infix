#!/usr/bin/env python3
#
# lo     br-0                    br-Q.40           br-D         br-X
#  |       |                       |                |            |
#  o       o      eth-Q.10       br-Q           veth0a.20     eth-X.30
#                         \     /    \              |            |
#                          eth-Q     veth0b       veth0a        eth-X
#                                         `---------'

"""
Verify that all interface types can be created

This test verify that all interface types can be created
and also, tesing setting the configuration in sequal (this
takes a little longer time than send it once)

"""

import infamy
import infamy.iface as iface


def verify_interface(target, interface, expected_type):
    assert iface.interface_exist(target, interface), f"Interface <{interface}> does not exist."

    expected_type = f"infix-if-type:{expected_type}"
    actual_type = iface._iface_get_param(target, interface, "type")

    if expected_type == "infix-if-type:etherlike"  and actual_type == "infix-if-type:ethernet":
        return  # Allow 'etherlike' to match 'ethernet'

    assert actual_type == expected_type, f"Assertion failed! expected tpye: {expected_type}, actual type {actual_type}"


with infamy.Test() as test:
    with test.step("Initialize"):
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

    with test.step("Configure an empty bridge"):
        target.put_config_dict("ietf-interfaces", {
        "interfaces": {
            "interface": [
                {
                    "name": br_0,
                    "type": "infix-if-type:bridge",
                    "enabled": True,
                }
            ]
        }
    })

    with test.step("Configure bridge brX and associated interfaces"):
        target.put_config_dict("ietf-interfaces", {
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
    })

    with test.step("Configure VETH pair"):
        target.put_config_dict("ietf-interfaces", {
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
        })

    with test.step("Configure bridge brD and associated interfaces"):
        target.put_config_dict("ietf-interfaces", {
        "interfaces": {
            "interface": [
                {
                    "name": br_D,
                    "type": "infix-if-type:bridge",
                    "enabled": True,
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
    })

    with test.step("Configure br-Q and associated interfaces"):
        target.put_config_dict("ietf-interfaces", {
        "interfaces": {
            "interface": [
                {
                    "name": br_Q,
                    "type": "infix-if-type:bridge",
                    "enabled": True,
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
    })

    with test.step("Verify interface 'lo' is of type loopback"):
        verify_interface(target, "lo", "loopback")

    with test.step("Verify interfaces 'ethX' and 'ethQ' is of type 'ethernet' (or etherlike if running Qemu)"):
         verify_interface(target, eth_X, "etherlike")
         verify_interface(target, eth_Q, "etherlike")

    with test.step("Verify interfaces 'br-0', 'br-X', 'br-D' and 'br-Q' is of type 'bridge'"):
        verify_interface(target, "br-0", "bridge")
        verify_interface(target, "br-X", "bridge")
        verify_interface(target, "br-Q", "bridge")
        verify_interface(target, "br-D", "bridge")

    with test.step("Verify interfaces 'veth0a' and 'veth0b' is of type 'veth'"):
        verify_interface(target, "veth0a", "veth")
        verify_interface(target, "veth0b", "veth")

    with test.step("Verify interfaces 'veth0a.20', 'ethQ.10', 'ethX.30', 'ethQ.10' and 'br-Q.40' is of type 'vlan'"):
        verify_interface(target, "veth0a.20", "vlan")
        verify_interface(target, f"{eth_X}.30", "vlan")
        verify_interface(target, f"{eth_Q}.10", "vlan")
        verify_interface(target, "br-Q.40", "vlan")

    test.succeed()
