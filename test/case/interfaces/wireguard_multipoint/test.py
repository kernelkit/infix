#!/usr/bin/env python3
r"""
Advanced WireGuard multipoint hub-and-spoke tunnel test

Set up a WireGuard hub-and-spoke topology with one server (hub) and three
clients (spokes). The server acts as a central point through which all clients
can communicate. A host is connected to client1 and can reach the server and
other clients through the WireGuard tunnel mesh.

This test verifies:

- WireGuard hub-and-spoke topology with multiple peers
- Advanced key management with preshared keys for post-quantum resistance
- Persistent keepalive configuration for NAT traversal
- Different listen ports on server and clients
- Multiple allowed-ips per peer for routing multiple subnets
- IPv4 and IPv6 connectivity through the encrypted tunnel mesh
- Proper routing between all nodes in the WireGuard network

Topology:
....
WireGuard Hub-and-Spoke (all clients connect to server):
                                         server:wg0
                                         10.0.0.1
                                            |
                        +-------------------+-------------------+
                        |                   |                   |
                  client1:wg0          client2:wg0          client3:wg0
                  10.0.0.10            10.0.0.20            10.0.0.30

....

"""

import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        server = env.attach("server", "mgmt")
        client1 = env.attach("client1", "mgmt")
        client2 = env.attach("client2", "mgmt")
        client3 = env.attach("client3", "mgmt")

    with test.step("Generate WireGuard keys and preshared keys"):
        # Server keys
        server_private_key = "6M5VmXgJ9qNzR8fZkR6QqHxKdJnP3bL2vN4sT7wB9Xk="
        server_public_key = "3kV8xR2YmN4pQ7wL5sJ9tZ1vB6nH8fK3dC2gA0qX4Rw="

        # Client1 keys
        client1_private_key = "aBcDeFgHiJkLmNoPqRsTuVwXyZ1234567890ABCDEF0="
        client1_public_key = "9ZyXwVuTsRqPoNmLkJiHgFeDcBa098765432ZYXWVUT="

        # Client2 keys
        client2_private_key = "2K9mXpL4vN7sR1tW5qZ8jH3gF6dA0cB9yX2eU4oI7nP="
        client2_public_key = "7PnI4oU2eXy9BcA0dF6gH3jZ8qW5tR1sN7vL4pXm9K2="

        # Client3 keys
        client3_private_key = "5QwErTyUiOpAsDfGhJkLzXcVbNm1234567890qWeRtY="
        client3_public_key = "1YtReWq098765432mNbVcXzLkJhGfDsApOiUyTrEwQ5="

        # Preshared keys (256-bit symmetric keys, base64-encoded)
        # Used for post-quantum resistance
        psk_client1 = "pZ3vN8mK5jH2gF4dS1aQ9wE7rT6yU0iO8pL3kJ4hG2f="
        psk_client2 = "xC4vB7nM2kJ9hG6fD3sA0qP8wE5rT2yU1iO4pL7kJ3h="
        psk_client3 = "mL9kJ6hG3fD0sA7qP4wE1rT8yU5iO2pL3kJ0hG9fD6s="

    with test.step("Configure WireGuard hub on server"):
        server.put_config_dicts({
            "ietf-keystore": {
                "keystore": {
                    "asymmetric-keys": {
                        "asymmetric-key": [{
                            "name": "server-wg-key",
                            "public-key-format": "infix-crypto-types:x25519-public-key-format",
                            "public-key": server_public_key,
                            "private-key-format": "infix-crypto-types:x25519-private-key-format",
                            "cleartext-private-key": server_private_key
                        }]
                    },
                    "symmetric-keys": {
                        "symmetric-key": [{
                            "name": "psk-client1",
                            "key-format": "infix-crypto-types:wireguard-symmetric-key-format",
                            "cleartext-key": psk_client1
                        }, {
                            "name": "psk-client2",
                            "key-format": "infix-crypto-types:wireguard-symmetric-key-format",
                            "cleartext-key": psk_client2
                        }, {
                            "name": "psk-client3",
                            "key-format": "infix-crypto-types:wireguard-symmetric-key-format",
                            "cleartext-key": psk_client3
                        }]
                    }
                }
            },
            "ietf-truststore": {
                "truststore": {
                    "public-key-bags": {
                        "public-key-bag": [{
                            "name": "wireguard-clients",
                            "public-key": [{
                                "name": "client1-wg-pubkey",
                                "public-key-format": "infix-crypto-types:x25519-public-key-format",
                                "public-key": client1_public_key
                            }, {
                                "name": "client2-wg-pubkey",
                                "public-key-format": "infix-crypto-types:x25519-public-key-format",
                                "public-key": client2_public_key
                            }, {
                                "name": "client3-wg-pubkey",
                                "public-key-format": "infix-crypto-types:x25519-public-key-format",
                                "public-key": client3_public_key
                            }]
                        }]
                    }
                }
            },
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": server["link1"],
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.50.1",
                                "prefix-length": 24
                            }]
                        },
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:3c4d:50::1",
                                "prefix-length": 64
                            }]
                        }
                    }, {
                        "name": server["link2"],
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.60.2",
                                "prefix-length": 24
                            }]
                        },
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:3c4d:60::2",
                                "prefix-length": 64
                            }]
                        }
                    }, {
                        "name": server["link3"],
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.70.3",
                                "prefix-length": 24
                            }]
                        },
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:3c4d:70::3",
                                "prefix-length": 64
                            }]
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
                            "private-key": "server-wg-key",
                            "peer": [{
                                "public-key-bag": "wireguard-clients",
                                "public-key": "client1-wg-pubkey",
                                "preshared-key": "psk-client1",
                                "endpoint": "192.168.50.10",
                                "endpoint-port": 51821,
                                "allowed-ips": ["10.0.0.10/32", "192.168.10.0/24", "fd00::10/128", "2001:db8:3c4d:10::/64"],
                                "persistent-keepalive": 25
                            }, {
                                "public-key-bag": "wireguard-clients",
                                "public-key": "client2-wg-pubkey",
                                "preshared-key": "psk-client2",
                                "endpoint": "192.168.60.20",
                                "endpoint-port": 51822,
                                "allowed-ips": ["10.0.0.20/32", "fd00::20/128"],
                                "persistent-keepalive": 30
                            }, {
                                "public-key-bag": "wireguard-clients",
                                "public-key": "client3-wg-pubkey",
                                "preshared-key": "psk-client3",
                                "endpoint": "192.168.70.30",
                                "endpoint-port": 51823,
                                "allowed-ips": ["10.0.0.30/32", "fd00::30/128"]
                            }]
                        }
                    }]
                }
            }
        })

    with test.step("Configure WireGuard spoke on client1"):
        client1.put_config_dicts({
            "ietf-keystore": {
                "keystore": {
                    "asymmetric-keys": {
                        "asymmetric-key": [{
                            "name": "client1-wg-key",
                            "public-key-format": "infix-crypto-types:x25519-public-key-format",
                            "public-key": client1_public_key,
                            "private-key-format": "infix-crypto-types:x25519-private-key-format",
                            "cleartext-private-key": client1_private_key
                        }]
                    },
                    "symmetric-keys": {
                        "symmetric-key": [{
                            "name": "psk-server",
                            "key-format": "infix-crypto-types:wireguard-symmetric-key-format",
                            "cleartext-key": psk_client1
                        }]
                    }
                }
            },
            "ietf-truststore": {
                "truststore": {
                    "public-key-bags": {
                        "public-key-bag": [{
                            "name": "wireguard-server",
                            "public-key": [{
                                "name": "server-wg-pubkey",
                                "public-key-format": "infix-crypto-types:x25519-public-key-format",
                                "public-key": server_public_key
                            }]
                        }]
                    }
                }
            },
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": client1["link"],
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.50.10",
                                "prefix-length": 24
                            }]
                        },
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:3c4d:50::10",
                                "prefix-length": 64
                            }]
                        }
                    }, {
                        "name": client1["data"],
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
                                "ip": "10.0.0.10",
                                "prefix-length": 24
                            }],
                            "forwarding": True
                        },
                        "ipv6": {
                            "address": [{
                                "ip": "fd00::10",
                                "prefix-length": 64
                            }],
                            "forwarding": True
                        },
                        "wireguard": {
                            "listen-port": 51821,
                            "private-key": "client1-wg-key",
                            "peer": [{
                                "public-key-bag": "wireguard-server",
                                "public-key": "server-wg-pubkey",
                                "preshared-key": "psk-server",
                                "endpoint": "192.168.50.1",
                                "endpoint-port": 51820,
                                "allowed-ips": ["10.0.0.0/24", "fd00::/64"],
                                "persistent-keepalive": 25
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
                                        "destination-prefix": "10.0.0.0/24",
                                        "next-hop": {
                                            "next-hop-address": "10.0.0.1"
                                        }
                                    }]
                                },
                                "ipv6": {
                                    "route": [{
                                        "destination-prefix": "fd00::/64",
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
        })

    with test.step("Configure WireGuard spoke on client2"):
        client2.put_config_dicts({
            "ietf-keystore": {
                "keystore": {
                    "asymmetric-keys": {
                        "asymmetric-key": [{
                            "name": "client2-wg-key",
                            "public-key-format": "infix-crypto-types:x25519-public-key-format",
                            "public-key": client2_public_key,
                            "private-key-format": "infix-crypto-types:x25519-private-key-format",
                            "cleartext-private-key": client2_private_key
                        }]
                    },
                    "symmetric-keys": {
                        "symmetric-key": [{
                            "name": "psk-server",
                            "key-format": "infix-crypto-types:wireguard-symmetric-key-format",
                            "cleartext-key": psk_client2
                        }]
                    }
                }
            },
            "ietf-truststore": {
                "truststore": {
                    "public-key-bags": {
                        "public-key-bag": [{
                            "name": "wireguard-server",
                            "public-key": [{
                                "name": "server-wg-pubkey",
                                "public-key-format": "infix-crypto-types:x25519-public-key-format",
                                "public-key": server_public_key
                            }]
                        }]
                    }
                }
            },
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": client2["link"],
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.60.20",
                                "prefix-length": 24
                            }]
                        },
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:3c4d:60::20",
                                "prefix-length": 64
                            }]
                        }
                    }, {
                        "name": "wg0",
                        "type": "infix-if-type:wireguard",
                        "ipv4": {
                            "address": [{
                                "ip": "10.0.0.20",
                                "prefix-length": 24
                            }]
                        },
                        "ipv6": {
                            "address": [{
                                "ip": "fd00::20",
                                "prefix-length": 64
                            }]
                        },
                        "wireguard": {
                            "listen-port": 51822,
                            "private-key": "client2-wg-key",
                            "peer": [{
                                "public-key-bag": "wireguard-server",
                                "public-key": "server-wg-pubkey",
                                "preshared-key": "psk-server",
                                "endpoint": "192.168.60.2",
                                "endpoint-port": 51820,
                                "allowed-ips": ["10.0.0.0/24", "192.168.10.0/24", "fd00::/64", "2001:db8:3c4d:10::/64"],
                                "persistent-keepalive": 30
                            }]
                        }
                    }]
                }
            }
        })

    with test.step("Configure WireGuard spoke on client3"):
        client3.put_config_dicts({
            "ietf-keystore": {
                "keystore": {
                    "asymmetric-keys": {
                        "asymmetric-key": [{
                            "name": "client3-wg-key",
                            "public-key-format": "infix-crypto-types:x25519-public-key-format",
                            "public-key": client3_public_key,
                            "private-key-format": "infix-crypto-types:x25519-private-key-format",
                            "cleartext-private-key": client3_private_key
                        }]
                    },
                    "symmetric-keys": {
                        "symmetric-key": [{
                            "name": "psk-server",
                            "key-format": "infix-crypto-types:wireguard-symmetric-key-format",
                            "cleartext-key": psk_client3
                        }]
                    }
                }
            },
            "ietf-truststore": {
                "truststore": {
                    "public-key-bags": {
                        "public-key-bag": [{
                            "name": "wireguard-server",
                            "public-key": [{
                                "name": "server-wg-pubkey",
                                "public-key-format": "infix-crypto-types:x25519-public-key-format",
                                "public-key": server_public_key
                            }]
                        }]
                    }
                }
            },
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": client3["link"],
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.70.30",
                                "prefix-length": 24
                            }]
                        },
                        "ipv6": {
                            "address": [{
                                "ip": "2001:db8:3c4d:70::30",
                                "prefix-length": 64
                            }]
                        }
                    }, {
                        "name": "wg0",
                        "type": "infix-if-type:wireguard",
                        "ipv4": {
                            "address": [{
                                "ip": "10.0.0.30",
                                "prefix-length": 24
                            }]
                        },
                        "ipv6": {
                            "address": [{
                                "ip": "fd00::30",
                                "prefix-length": 64
                            }]
                        },
                        "wireguard": {
                            "listen-port": 51823,
                            "private-key": "client3-wg-key",
                            "peer": [{
                                "public-key-bag": "wireguard-server",
                                "public-key": "server-wg-pubkey",
                                "preshared-key": "psk-server",
                                "endpoint": "192.168.70.3",
                                "endpoint-port": 51820,
                                "allowed-ips": ["10.0.0.0/24", "fd00::/64"]
                            }]
                        }
                    }]
                }
            }
        })

    _, hport = env.ltop.xlate("host", "data")
    with test.step("Verify IPv4 connectivity from host to server through client1"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("192.168.10.2")
            ns0.addroute("10.0.0.0/24", "192.168.10.1")
            ns0.must_reach("10.0.0.1")

    with test.step("Verify IPv4 connectivity from host to client2 through WireGuard mesh"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("192.168.10.2")
            ns0.addroute("10.0.0.0/24", "192.168.10.1")
            ns0.must_reach("10.0.0.20")

    with test.step("Verify IPv4 connectivity from host to client3 through WireGuard mesh"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("192.168.10.2")
            ns0.addroute("10.0.0.0/24", "192.168.10.1")
            ns0.must_reach("10.0.0.30")

    with test.step("Verify IPv6 connectivity from host to server through client1"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("2001:db8:3c4d:10::2", prefix_length=64, proto="ipv6")
            ns0.addroute("fd00::/64", "2001:db8:3c4d:10::1", proto="ipv6")
            ns0.must_reach("fd00::1")

    with test.step("Verify IPv6 connectivity from host to client2 through WireGuard mesh"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("2001:db8:3c4d:10::2", prefix_length=64, proto="ipv6")
            ns0.addroute("fd00::/64", "2001:db8:3c4d:10::1", proto="ipv6")
            ns0.must_reach("fd00::20")

    with test.step("Verify IPv6 connectivity from host to client3 through WireGuard mesh"):
        with infamy.IsolatedMacVlan(hport) as ns0:
            ns0.addip("2001:db8:3c4d:10::2", prefix_length=64, proto="ipv6")
            ns0.addroute("fd00::/64", "2001:db8:3c4d:10::1", proto="ipv6")
            ns0.must_reach("fd00::30")

    test.succeed()
