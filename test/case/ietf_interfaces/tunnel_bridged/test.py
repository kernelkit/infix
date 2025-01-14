#!/usr/bin/env python3
"""
Tunnel interface bridged with physical

Test that {type} works as it should and that it possible to bridge it.

"""

import infamy

class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.add_argument("--type")

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        args=ArgumentParser()
        env = infamy.Env(args=args)
        left = env.attach("left", "mgmt")
        right = env.attach("right", "mgmt")
        type = env.args.type

    with test.step("Configure DUTs"):
        if type == "gretap":
            container_name = "gre"
        else:
            container_name = type

        container_left =  {
            "local": "192.168.50.1",
            "remote": "192.168.50.2"
        }
        container_right =  {
            "local": "192.168.50.2",
            "remote": "192.168.50.1"
        }
        container_left.update({
            "vni": 4
        })
        container_right.update({
            "vni": 4
        })
        left.put_config_dicts({ "ietf-interfaces": {
            "interfaces": {
                "interface": [
                {
                    "name": left["link"],
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.50.1",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    }
                },
                {
                    "name": left["data"],
                    "bridge-port": {
                        "bridge": "br0"
                    }
                },
                {
                    "name": "br0",
                    "type": "infix-if-type:bridge"
                },
                {
                    "name": f"{type}0",
                    "type": f"infix-if-type:{type}",
                    container_name: container_left,
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
                        "name": right["link"],
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
                        "name": f"{type}0",
                        "type": f"infix-if-type:{type}",
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.10.2",
                                "prefix-length": 24
                            }],
                            "forwarding": True
                        },
                        container_name: container_right,
                    }]
                }
            }
        })
    _, hport = env.ltop.xlate("host", "data")
    with test.step(f"Test connectivity host:data to right:{type}0 at 192.168.10.2"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("192.168.10.1")
            ns0.must_reach("192.168.10.2")

    test.succeed()
