#!/usr/bin/env python3
r"""
Advanced WireGuard multipoint hub-and-spoke tunnel test

Set up a WireGuard hub-and-spoke topology with one server (hub) and two
clients (spokes). The server acts as a central point through which clients
can communicate. Host namespaces are connected behind the server and client1
to test routing through the WireGuard mesh.

This test verifies:

- WireGuard hub-and-spoke topology with multiple peers
- Mixed IPv4/IPv6 tunnel endpoints (client1 uses IPv4, client2 uses IPv6)
- Dual-stack WireGuard tunnels carrying both IPv4 and IPv6 traffic
- Advanced key management with preshared keys for post-quantum resistance
- Persistent keepalive configuration for NAT traversal
- Different listen ports on server and clients
- Multiple allowed-ips per peer for routing multiple subnets
- Static routes for subnet reachability through WireGuard
- Security boundaries enforced by allowed-ips (client2 isolated from server subnet)
- IPv4 and IPv6 connectivity through the encrypted tunnel mesh
- Proper routing between all nodes in the WireGuard network

WireGuard hub-and-spoke:
....
                    server:wg0 (10.0.0.1, fd00:0::1)
                                 |
                +----------------+----------------+
                |                                 |
         client1:wg0                        client2:wg0
      (10.0.0.2, fd00:0::2)              (10.0.0.3, fd00:0::3)
      via IPv4 endpoint                  via IPv6 endpoint
      192.168.10.x                       2001:db8:3c4d:20::x
....

Security boundaries:
- host:data1 can reach all WireGuard IPs (10.0.0.1, .2, .3 and fd00:0::1, ::2, ::3)
- host:data2 can reach server and client1 WireGuard IPs, but NOT client2 (blocked by allowed-ips)

"""

import infamy
import infamy.util as util

# Server keys
server_private_key = "uIUL4AnD5QaVrwHDPHJzQ7sIQ+Q3zDdflnvfd59qa28="
server_public_key = "qGVmu5UbNtMuZs2t9wFoOoHlvgmV+A1SyQacVb/bEV0="

# Client1 keys
client1_private_key = "kNmkNlSkSh9+Va2tmFv9Va8TBCZlTBF0fKAGJf8vomo="
client1_public_key = "ROaZyvJc5DzA2XUAAeTj2YlwDsy2w0lr3t+rWj2imAk="

# Client2 keys
client2_private_key = "OPT7v/l5zICEmFIrO0U+YwA+w07l8Xo2Dp38hjGOHGY="
client2_public_key = "Om9CPLYdK3l93GauKrq5WXo/gbcD+1CeqFpobRLLkB4="

# Preshared keys (256-bit symmetric keys, base64-encoded)
psk_client1 = "zYr83O4Ykj9i1gN+/aaosJxQxCzvXv1EYOj0MX9H2K4="
psk_client2 = "A4Gf6KCp+CL+tH2TUd9cyARpBZAH8e+9QXiPJ0t+4So="


def configure_server(dut):
    dut.put_config_dicts({
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
                        "cleartext-symmetric-key": psk_client1,
                        "key-format": "ietf-crypto-types:octet-string-key-format"
                    }, {
                        "name": "psk-client2",
                        "cleartext-symmetric-key": psk_client2,
                        "key-format": "ietf-crypto-types:octet-string-key-format"
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
                            "ip": "192.168.10.1",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": server["link2"],
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:3c4d:20::1",
                            "prefix-length": 64
                        }]
                    }
                }, {
                    "name": server["data"],
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.0.2",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    },
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:3c4d:02::2",
                            "prefix-length": 64
                        }],
                        "forwarding": True
                    }
                },
                {
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
                            "ip": "fd00:0::1",
                            "prefix-length": 64
                        }],
                        "forwarding": True
                    },
                    "wireguard": {
                        "listen-port": 51820,
                        "private-key": "server-wg-key",
                        "peers": [{
                            "public-key-bag": "wireguard-clients",
                            "persistent-keepalive": 3,
                            "peer": [{
                                "public-key": "client1-wg-pubkey",
                                "preshared-key": "psk-client1",
                                "endpoint": "192.168.10.2",
                                "endpoint-port": 51821,
                                "allowed-ips": ["10.0.0.2/32", "192.168.1.0/24", "fd00:0::2/128", "2001:db8:3c4d:01::/64"]
                            }, {
                                "public-key": "client2-wg-pubkey",
                                "preshared-key": "psk-client2",
                                "endpoint": "2001:db8:3c4d:20::2",
                                "endpoint-port": 51822,
                                "allowed-ips": ["10.0.0.3/32", "fd00:0::3/128"],
                                "persistent-keepalive": 5
                            }]
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
                                    "destination-prefix": "192.168.1.0/24",
                                    "next-hop": {
                                        "next-hop-address": "10.0.0.2"
                                    }
                                }]
                            },
                            "ipv6": {
                                "route": [{
                                    "destination-prefix": "2001:db8:3c4d:01::/64",
                                    "next-hop": {
                                        "next-hop-address": "fd00:0::2"
                                    }
                                }]
                            }
                        }
                    }]
                }
            }
        }
    })

def configure_client1(dut):
    dut.put_config_dicts({
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
                        "cleartext-symmetric-key": psk_client1,
                        "key-format": "ietf-crypto-types:octet-string-key-format"
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
                            "ip": "192.168.10.2",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": client1["data"],
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.1.2",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    },
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:3c4d:01::2",
                            "prefix-length": 64
                        }],
                        "forwarding": True
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
                            "ip": "fd00:0::2",
                            "prefix-length": 64
                        }],
                        "forwarding": True
                    },
                    "wireguard": {
                        "listen-port": 51821,
                        "private-key": "client1-wg-key",
                        "peers": [{
                            "public-key-bag": "wireguard-server",
                            "preshared-key": "psk-server",
                            "endpoint": "192.168.10.1",
                            "endpoint-port": 51820,
                            "allowed-ips": ["10.0.0.1/32", "10.0.0.3/32", "192.168.0.0/24", "fd00:0::1/128", "fd00:0::3/128", "2001:db8:3c4d:02::/64"],
                            "persistent-keepalive": 3
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
                                }, {
                                    "destination-prefix": "192.168.0.0/16",
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
                                }, {
                                    "destination-prefix": "2001:db8:3c4d:02::/64",
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

def configure_client2(dut):
    dut.put_config_dicts({
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
                        "cleartext-symmetric-key": psk_client2,
                        "key-format": "ietf-crypto-types:octet-string-key-format"
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
                    "ipv6": {
                        "address": [{
                            "ip": "2001:db8:3c4d:20::2",
                            "prefix-length": 64
                        }]
                    }
                }, {
                    "name": "wg0",
                    "type": "infix-if-type:wireguard",
                    "ipv4": {
                        "address": [{
                            "ip": "10.0.0.3",
                            "prefix-length": 24
                        }]
                    },
                    "ipv6": {
                        "address": [{
                            "ip": "fd00:0::3",
                            "prefix-length": 64
                        }]
                    },

                    "wireguard": {
                        "listen-port": 51822,
                        "private-key": "client2-wg-key",
                        "peers": [{
                            "public-key-bag": "wireguard-server",
                            "preshared-key": "psk-server",
                            "endpoint": "2001:db8:3c4d:20::1",
                            "endpoint-port": 51820,
                            "allowed-ips": ["10.0.0.1/32", "10.0.0.2/32", "192.168.1.0/24", "fd00:0::1/128", "fd00:0::2/128", "2001:db8:3c4d:01::/64"],
                            "persistent-keepalive": 5
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
                                }, {
                                    "destination-prefix": "192.168.0.0/16",
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
                                }, {
                                    "destination-prefix": "2001:db8:3c4d:01::/64",
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

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        server = env.attach("server", "mgmt")
        client1 = env.attach("client1", "mgmt")
        client2 = env.attach("client2", "mgmt")

    _, hclient1 = env.ltop.xlate("host", "data1")
    _, hserver  = env.ltop.xlate("host", "data2")

    with test.step("Configure server, client1 and client2"):
        util.parallel(configure_server(server), configure_client1(client1), configure_client2(client2))

    with infamy.IsolatedMacVlan(hserver) as nsserver, infamy.IsolatedMacVlan(hclient1) as nsclient1:
        nsserver.addip("192.168.0.1")
        nsserver.addroute("default", "192.168.0.2");
        nsclient1.addip("192.168.1.1")
        nsclient1.addroute("default", "192.168.1.2");
        nsserver.addip("2001:db8:3c4d:02::100", prefix_length=64, proto="ipv6")
        nsserver.addroute("default", "2001:db8:3c4d:02::2", proto="ipv6")
        nsclient1.addip("2001:db8:3c4d:01::100", prefix_length=64, proto="ipv6")
        nsclient1.addroute("default", "2001:db8:3c4d:01::2", proto="ipv6")

        with test.step("Verify IPv4 connectivity with ping 10.0.0.1, 10.0.0.2 and 10.0.0.3 from host:data1"):
            util.parallel(nsclient1.must_reach("10.0.0.1"),
                          nsclient1.must_reach("10.0.0.2"),
                          nsclient1.must_reach("10.0.0.3"))

        with test.step("Verify IPv4 connectivity with ping 10.0.0.1 and 10.0.0.2 from host:data2"):
            util.parallel(nsserver.must_reach("10.0.0.1"),
                          nsserver.must_reach("10.0.0.2"))

        with test.step("Verify host:data2 can not ping 10.0.0.3"):
            nsserver.must_not_reach("10.0.0.3") # Not in allowed IPs

        with test.step("Verify IPv6 connectivity with ping fd00:0::1, fd00:0::2 and fd00:0::3 from host:data1"):
            util.parallel(nsclient1.must_reach("fd00:0::1"),
                          nsclient1.must_reach("fd00:0::2"),
                          nsclient1.must_reach("fd00:0::3"))

        with test.step("Verify IPv6 connectivity with ping fd00:0:1 and fd00:0:2 from host:data2"):
            util.parallel(nsserver.must_reach("fd00:0::1"),
                          nsserver.must_reach("fd00:0::2"))

        with test.step("Verify host:data2 can not ping fd00:0::3"):
            nsserver.must_not_reach("fd00:0::3") # Not in allowed IPs


    test.succeed()
