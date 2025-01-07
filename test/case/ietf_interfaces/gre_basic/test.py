#!/usr/bin/env python3
"""
Basic IP GRE test

Test setting up IP GRE tunnels using IPv4 and IPv6,
and then a connectivity test.
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
                    "name": leftdata,
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
                    "name": "gre0",
                    "type": "infix-if-type:gre",
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.30.1",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    },
                    "gre": {
                        "local": "192.168.50.1",
                        "remote": "192.168.50.2"
                    }

                },
                {
                    "name": "gre6",
                    "type": "infix-if-type:gre",
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:3c4d:30::1",
                            "prefix-length": 64
                        }]
                    },
                    "gre": {
                        "local": "2001:db8:3c4d:50::1",
                        "remote": "2001:db8:3c4d:50::2",
                    }
                }]
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
                        "name": "gre1",
                        "type": "infix-if-type:gre",
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.30.2",
                                "prefix-length": 24
                            }],
                            "forwarding": True
                        },
                        "gre": {
                            "local": "192.168.50.2",
                            "remote": "192.168.50.1"
                        }
                    },
                    {
                        "name": "gre6",
                        "type": "infix-if-type:gre",
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:3c4d:30::2",
                                "prefix-length": 64
                            }]
                        },
                        "gre": {
                            "local": "2001:db8:3c4d:50::2",
                            "remote": "2001:db8:3c4d:50::1",
                        }
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
    with test.step("Test connectivity host:data to gre 10.0.0.2"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("192.168.10.2")
            ns0.addroute("192.168.30.0/24", "192.168.10.1")
            ns0.must_reach("192.168.30.2")
    with test.step("Test connectivity host:data to gre on right 2001:db8::c0a8:0a02"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("2001:db8:3c4d:10::2", prefix_length=64, proto="ipv6")
            ns0.addroute("2001:db8:3c4d:30::/64", "2001:db8:3c4d:10::1", proto="ipv6")
            #breakpoint()
            ns0.must_reach("2001:db8:3c4d:30::2")

    test.succeed()
