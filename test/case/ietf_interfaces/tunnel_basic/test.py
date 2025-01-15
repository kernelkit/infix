#!/usr/bin/env python3
"""
Basic tunnel connectivity test

Test setting up {type} tunnels using IPv4 and IPv6,
and ends with a connectivity test.
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
        type = env.args.type
        left = env.attach("left", "mgmt")
        right = env.attach("right", "mgmt")

    with test.step("Configure DUTs"):
        container_left4 =  {
            "local": "192.168.50.1",
            "remote": "192.168.50.2"
        }
        container_left6 = {
            "local": "2001:db8:3c4d:50::1",
            "remote": "2001:db8:3c4d:50::2",
        }
        container_right4 =  {
            "local": "192.168.50.2",
            "remote": "192.168.50.1"
        }
        container_right6 = {
            "local": "2001:db8:3c4d:50::2",
            "remote": "2001:db8:3c4d:50::1",
        }
        if type == "gretap" or type == "gre":
            container_name = "gre"
        if type == "vxlan":
            container_name = "vxlan"
            container_left4.update({
                "vni": 4
            })
            container_left6.update({
                "vni": 6
            })
            container_right4.update({
                "vni": 4
            })
            container_right6.update({
                "vni": 6
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
                    },
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:3c4d:50::1",
                            "prefix-length": 64
                        }],
                        "forwarding": True
                    }
                },
                {
                    "name": left["data"],
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.10.1",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    },
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:3c4d:10::1",
                            "prefix-length": 64
                        }],
                        "forwarding": True
                    }
                },
                {
                    "name": f"{type}4",
                    "type": f"infix-if-type:{type}",
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.30.1",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    },
                    container_name: container_left4
                },
                {
                    "name": f"{type}6",
                    "type": f"infix-if-type:{type}",
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:3c4d:30::1",
                            "prefix-length": 64
                        }]
                    },
                    container_name: container_left6
                }]
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
                        "name": f"{type}4",
                        "type": f"infix-if-type:{type}",
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.30.2",
                                "prefix-length": 24
                            }],
                            "forwarding": True
                        },
                        container_name: container_right4
                    },
                    {
                        "name": f"{type}",
                        "type": f"infix-if-type:{type}",
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:3c4d:30::2",
                                "prefix-length": 64
                            }]
                        },
                        container_name: container_right6
                    }]
                }
            },
            "ietf-routing": {
                "routing": {
                    "control-plane-protocols": {
                        "control-plane-protocol": [{
                            "type": "infix-routing:static",
                            "name": "default",
                            "static-routes": {
                                "ipv4": {
                                    "route": [{
                                        "destination-prefix": "192.168.10.0/24",
                                        "next-hop": {
                                            "next-hop-address": "192.168.30.1"
                                        }
                                    }]
                                },
                                "ipv6": {
                                    "route": [{
                                        "destination-prefix": "2001:db8:3c4d:10::/64",
                                        "next-hop": {
                                            "next-hop-address": "2001:db8:3c4d:30::1"
                                        }
                                    }]
                                }
                            }
                        }]
                    }
                }
            }
        })
    _, hport = env.ltop.xlate("host", "data")
    with test.step("Verify connectivity host:data to 10.0.0.2"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("192.168.10.2")
            ns0.addroute("192.168.30.0/24", "192.168.10.1")
            ns0.must_reach("192.168.30.2")
    with test.step("Verify connectivity host:data to 2001:db8::c0a8:0a02"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("2001:db8:3c4d:10::2", prefix_length=64, proto="ipv6")
            ns0.addroute("2001:db8:3c4d:30::/64", "2001:db8:3c4d:10::1", proto="ipv6")
            ns0.must_reach("2001:db8:3c4d:30::2")

    test.succeed()
