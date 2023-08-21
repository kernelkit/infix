#!/usr/bin/env python3
#
#                vlan10 IP:10.0.0.2
#                /
# PING -->     br0  <-- VLAN filtering enabled
#             /
#   PC ---- e0
#

import infamy

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x2"))
        target = env.attach("target", "mgmt")

    with test.step("Configure single vlan-filtering bridge with vlan10 subinterface @ IP 10.0.0.2"):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
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
                                        "untagged": [ tport ],
                                        "tagged":   [ "br0" ]
                                    }
                                ]
                            }
                        }
                    },
                    {
                        "name": "vlan10",
                        "type": "infix-if-type:vlan",
                        "enabled": True,
                        "parent-interface": "br0",
                        "encapsulation": {
                            "dot1q-vlan": {
                                "outer-tag": {
                                    "tag-type": "ieee802-dot1q-types:c-vlan",
                                    "vlan-id": 10,
                                }
                            }
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
                        "name": tport,
                        "type": "infix-if-type:ethernet",
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "pvid": 10,
                            "bridge": "br0"
                        }
                    },
                ]
            }
        })

    with test.step("Ping vlan10 subinterface 10.0.0.2 from host:data with IP 10.0.0.1"):
        _, hport = env.ltop.xlate("host", "data")

        with infamy.IsolatedMacVlan(hport) as ns:
            pingtest = ns.runsh("""
            set -ex

            ip link set iface up
            ip addr add 10.0.0.1/24 dev iface

            ping -c1 -w5 10.0.0.2 || exit 1
            """)

        if pingtest.returncode:
            print(pingtest.stdout)
            test.fail()

    test.succeed()
