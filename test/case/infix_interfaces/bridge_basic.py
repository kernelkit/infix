#!/usr/bin/env python3
#
# PING -->     br0      br1 10.0.0.2
#             /   \    /
#   PC ---- e0     veth
#

import infamy

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x2"))
        target = env.attach("target", "mgmt")

    with test.step("Configure two bridges linked with a veth pair furthest bridge has IP 10.0.0.2"):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": "br0",
                        "type": "iana-if-type:bridge",
                        "enabled": True,
                    },
                    {
                        "name": "br1",
                        "type": "iana-if-type:bridge",
                        "enabled": True,
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
                        "name": "veth0a",
                        "type": "infix-if-type:veth",
                        "enabled": True,
                        "infix-interfaces:veth": {
                            "peer": "veth0b"
                        },
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                    {
                        "name": "veth0b",
                        "type": "infix-if-type:veth",
                        "enabled": True,
                        "infix-interfaces:veth": {
                            "peer": "veth0a"
                        },
                        "infix-interfaces:bridge-port": {
                            "bridge": "br1"
                        }
                    },
                    {
                        "name": tport,
                        "type": "iana-if-type:ethernetCsmacd",
                        "enabled": True,
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    },
                ]
            }
        })

    with test.step("Ping furthest bridge 10.0.0.2 from host:data with IP 10.0.0.1"):
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
