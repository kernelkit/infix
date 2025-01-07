#!/usr/bin/env python3
"""
GRETAP interface bridged with physical

Test that GRETAP works as it should and that it possible to bridge it.

"""

import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
           env = infamy.Env()
           left = env.attach("left", "mgmt")
           right = env.attach("right", "mgmt")
           _, leftlink = env.ltop.xlate("left", "link")
           _, leftdata = env.ltop.xlate("left", "data")
           _, rightlink = env.ltop.xlate("right", "link")


    with test.step("Configure DUTs"):
        left.put_config_dicts({ "ietf-interfaces": {
            "interfaces": {
                "interface": [
                {
                    "name": leftlink,
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.50.1",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    }
                },
                {
                    "name": leftdata,
                    "bridge-port": {
                        "bridge": "br0"
                    }
                },
                {
                    "name": "br0",
                    "type": "infix-if-type:bridge"
                },
                {
                    "name": "gre0",
                    "type": "infix-if-type:gretap",
                    "gre": {
                        "local": "192.168.50.1",
                        "remote": "192.168.50.2"
                    },
                    "bridge-port": {
                        "bridge": "br0"
                    }

                }
            ]
        }
        }
    })

        right.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                    {
                        "name": rightlink,
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.50.2",
                                "prefix-length": 24
                            }],
                            "forwarding": True
                        },
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:3c4d:50::2",
                                "prefix-length": 64
                            }]
                        }
                    },
                    {
                        "name": "gre0",
                        "type": "infix-if-type:gretap",
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.10.2",
                                "prefix-length": 24
                            }],
                            "forwarding": True
                        },
                        "gre": {
                            "local": "192.168.50.2",
                            "remote": "192.168.50.1"
                        }
                    }]
                }
            }
        })
    _, hport = env.ltop.xlate("host", "data")
    with test.step("Test connectivity host:data to right:gre0 at 192.168.10.2"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("192.168.10.1")
            ns0.must_reach("192.168.10.2")

    test.succeed()
