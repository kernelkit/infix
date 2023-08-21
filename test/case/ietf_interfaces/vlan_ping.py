#!/usr/bin/env python3

import infamy

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x2"))
        target = env.attach("target", "mgmt")

    with test.step("Configure VLAN 10 on target:data with IP 10.0.0.2"):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": tport,
                        "type": "infix-if-type:ethernet",
                        "enabled": True,
                    },
                    {
                        "name": f"{tport}.10",
                        "type": "infix-if-type:vlan",
                        "parent-interface": tport,
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
                ]
            }
        })

    with test.step("Ping 10.0.0.2 from VLAN 10 on host:data with IP 10.0.0.1"):
        _, hport = env.ltop.xlate("host", "data")

        with infamy.IsolatedMacVlan(hport) as ns:
            pingtest = ns.runsh("""
            set -ex

            ip link set iface up
            ip link add dev vlan10 link iface up type vlan id 10
            ip addr add 10.0.0.1/24 dev vlan10

            ping -c1 -w5 10.0.0.2 || exit 1
            """)

        if pingtest.returncode:
            print(pingtest.stdout)
            test.fail()

    test.succeed()
