#!/usr/bin/env python3
"""
Tunnel interface bridged with physical

Set up a layer-2 tunnel between two DUTs, host is connected to the first DUT.
Bridge the interface connected to the host and the tunnel interface on the
first DUT.  On host, verify connectivity with the second DUT through tunnel.

- Tunnel types: GRETAP, and VxLAN
- Connectivity: IPv4
"""

import infamy


class ArgumentParser(infamy.ArgumentParser):
    def __init__(self):
        super().__init__()
        self.add_argument("--tunnel")


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        arg = ArgumentParser()
        env = infamy.Env(args=arg)
        left = env.attach("left", "mgmt")
        right = env.attach("right", "mgmt")
        tunnel = env.args.tunnel

    with test.step("Configure DUTs with tunnel {tunnel}"):
        if tunnel == "gretap":
            CONTAINER_TYPE = "gre"
        else:
            CONTAINER_TYPE = tunnel

        container_left = {
            "local": "192.168.50.1",
            "remote": "192.168.50.2"
        }
        container_right = {
            "local": "192.168.50.2",
            "remote": "192.168.50.1"
        }
        container_left.update({
            "vni": 4
        })
        container_right.update({
            "vni": 4
        })

        left.put_config_dicts({"ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": left["link"],
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.50.1",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    }
                }, {
                    "name": left["data"],
                    "bridge-port": {
                        "bridge": "br0"
                    }
                }, {
                    "name": "br0",
                    "type": "infix-if-type:bridge"
                }, {
                    "name": f"{tunnel}0",
                    "type": f"infix-if-type:{tunnel}",
                    CONTAINER_TYPE: container_left,
                    "bridge-port": {
                        "bridge": "br0"
                    }

                }]
            }
        }})

        right.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
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
                    }, {
                        "name": f"{tunnel}0",
                        "type": f"infix-if-type:{tunnel}",
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.10.2",
                                "prefix-length": 24
                            }],
                            "forwarding": True
                        },
                        CONTAINER_TYPE: container_right,
                    }]
                }
            }
        })

    _, hport = env.ltop.xlate("host", "data")
    with test.step(f"Test connectivity host:data to right:{tunnel}0 at 192.168.10.2"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("192.168.10.1")
            ns0.must_reach("192.168.10.2")

    test.succeed()
