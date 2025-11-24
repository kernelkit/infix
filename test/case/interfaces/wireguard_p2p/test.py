#!/usr/bin/env python3
r"""
Basic WireGuard point-to-point tunnel test

Set up a WireGuard tunnel between two DUTs with a host connected to the first
DUT. Enable IP forwarding on first DUT's interface to host and the WireGuard
tunnel interface to the second DUT. On host, add route to IP network of second
DUT and verify connectivity with the second DUT through the WireGuard tunnel.

This test verifies:

- WireGuard tunnel establishment between two peers
- Key management via ietf-keystore with X25519 keypairs
- IPv4 and IPv6 connectivity through the encrypted tunnel
- Proper routing through the WireGuard tunnel

Topology:
....
                     192.168.50.0/24
    host:data ---- left:data   left:link ---- right:link
 192.168.10.2/24  192.168.10.1  192.168.50.1   192.168.50.2
                                      \\              /
                                       \\            /
                                        \\  WireGuard
                                         \\ Tunnel  /
                                          \\      /
                                   left:wg0    right:wg0
                                 10.0.0.1/32  10.0.0.2/32

....

"""

import infamy
import infamy.util as util

left_private_key = "EJPoi0BnccsfjEhKk0IWwNzJKXZKgS6XaKt+InYITVA="
left_public_key = "xWVOEFUZZ5VI6t1fhZeISNyw7Ma/bY8INzIoaSSLlz8="

right_private_key = "UEaX13FTGhiIrnnKRd20KWh/vG6zqRIMSTzOP3hNs2s="
right_public_key = "2pytpunN+e3V9e5asMXP+UqKoerFm08KWzcFYoWP41k="

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        left = env.attach("left", "mgmt")
        right = env.attach("right", "mgmt")

    with test.step("Configure WireGuard tunnel on DUTs"):
        util.parallel(left.put_config_dicts({
            "ietf-keystore": {
                "keystore": {
                    "asymmetric-keys": {
                        "asymmetric-key": [{
                            "name": "left-wg-key",
                            "public-key-format": "infix-crypto-types:x25519-public-key-format",
                            "public-key": left_public_key,
                            "private-key-format": "infix-crypto-types:x25519-private-key-format",
                            "cleartext-private-key": left_private_key
                        }]
                    }
                }
            },
            "ietf-truststore": {
                "truststore": {
                    "public-key-bags": {
                        "public-key-bag": [{
                            "name": "wireguard-peers",
                            "public-key": [{
                                "name": "right-wg-pubkey",
                                "public-key-format": "infix-crypto-types:x25519-public-key-format",
                                "public-key": right_public_key
                            }]
                        }]
                    }
                }
            },
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
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
                    }, {
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
                    }, {
                        "name": "wg0",
                        "type": "infix-if-type:wireguard",
                        "ipv4": {
                            "address": [{
                                "ip": "10.0.0.1",
                                "prefix-length": 24
                            }],
                            "forwarding": True
                        },
                        "ipv6": {
                            "address": [{
                                "ip": "fd00::1",
                                "prefix-length": 64
                            }],
                            "forwarding": True
                        },
                        "wireguard": {
                            "listen-port": 51820,
                            "private-key": "left-wg-key",
                            "peers": [{
                                "public-key-bag": "wireguard-peers",
                                "endpoint": "192.168.50.2",
                                "endpoint-port": 51820,
                                "allowed-ips": ["10.0.0.0/24", "fd00::/64"]
                            }]
                        }
                    }]
                }
            }
        }),
        right.put_config_dicts({
            "ietf-keystore": {
                "keystore": {
                    "asymmetric-keys": {
                        "asymmetric-key": [{
                            "name": "right-wg-key",
                            "public-key-format": "infix-crypto-types:x25519-public-key-format",
                            "public-key": right_public_key,
                            "private-key-format": "infix-crypto-types:x25519-private-key-format",
                            "cleartext-private-key": right_private_key
                        }]
                    }
                }
            },
            "ietf-truststore": {
                "truststore": {
                    "public-key-bags": {
                        "public-key-bag": [{
                            "name": "wireguard-peers",
                            "public-key": [{
                                "name": "left-wg-pubkey",
                                "public-key-format": "infix-crypto-types:x25519-public-key-format",
                                "public-key": left_public_key
                            }]
                        }]
                    }
                }
            },
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
                        "name": "wg0",
                        "type": "infix-if-type:wireguard",
                        "ipv4": {
                            "address": [{
                                "ip": "10.0.0.2",
                                "prefix-length": 24
                            }],
                            "forwarding": True
                        },
                        "ipv6": {
                            "address": [{
                                "ip": "fd00::2",
                                "prefix-length": 64
                            }]
                        },
                        "wireguard": {
                            "listen-port": 51820,
                            "private-key": "right-wg-key",
                            "peers": [{
                                "public-key-bag": "wireguard-peers",
                                "endpoint": "192.168.50.1",
                                "endpoint-port": 51820,
                                "allowed-ips": ["10.0.0.0/24", "fd00::/64", "2001:db8:3c4d:10::/64", "192.168.10.0/24"]
                            }]
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
                                            "next-hop-address": "10.0.0.1"
                                        }
                                    }]
                                },
                                "ipv6": {
                                    "route": [{
                                        "destination-prefix": "2001:db8:3c4d:10::/64",
                                        "next-hop": {
                                            "next-hop-address": "fd00::1"
                                        }
                                    }]
                                }
                            }
                        }]
                    }
                }
            }
        }))

    _, hport = env.ltop.xlate("host", "data")
    with test.step("Verify IPv4 connectivity with ping 10.0.0.2 from host:data"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("192.168.10.2")
            ns0.addroute("10.0.0.0/24", "192.168.10.1")
            ns0.must_reach("10.0.0.2")

    with test.step("Verify IPv6 connectivity with ping fd00::2 from host:data"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("2001:db8:3c4d:10::2", prefix_length=64, proto="ipv6")
            ns0.addroute("fd00::/64", "2001:db8:3c4d:10::1", proto="ipv6")
            ns0.must_reach("fd00::2")

    test.succeed()
