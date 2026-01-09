#!/usr/bin/env python3
r"""
WireGuard roadwarrior test with router simulating internet

This test demonstrates a realistic roadwarrior scenario where clients
connect through an internet router to reach a VPN server.

Set up a WireGuard server with 2 roadwarrior clients connecting through
a router that simulates the internet. Each client is on a different subnet
behind the router, and has a private network that should be accessible
through the VPN tunnel.

This test verifies:

- WireGuard tunnel establishment through an intermediate router
- Roadwarrior clients on different subnets
- Access to server's private network through the VPN tunnel
- Routing between client networks and server network via WireGuard

Topology:
....
    Server ---- Router ---- Client1
    (VPN)    (Internet)     \---- Client2

    WAN: 192.168.100.0/24
    Server: 192.168.100.1
    Router: 192.168.100.2

    Client1 link: 192.168.50.0/24 (Router LAN1)
    Client2 link: 192.168.51.0/24 (Router LAN2)

    WireGuard tunnel: 10.0.0.0/24
    Server: 10.0.0.1/24
    Client1: 10.0.0.2/24
    Client2: 10.0.0.3/24

    Backend networks:
    Server data: 192.168.0.0/24
    Client1 data: 192.168.1.0/24
    Client2 data: 192.168.2.0/24
....

"""

import infamy
import infamy.util as util
import infamy.iface as iface
import infamy.wireguard as wg

def configure_server(server, server_public_key, server_private_key, client1_public_key, client2_public_key):
    """Configure WireGuard server with key-bag level settings for all clients"""
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
                }
            }
        },
        "ietf-truststore": {
            "truststore": {
                "public-key-bags": {
                    "public-key-bag": [{
                        "name": "roadwarriors",
                        "public-key": [{
                            "name": "client1-key",
                            "public-key-format": "infix-crypto-types:x25519-public-key-format",
                            "public-key": client1_public_key,
                        }, {
                            "name": "client2-key",
                            "public-key-format": "infix-crypto-types:x25519-public-key-format",
                            "public-key": client2_public_key,
                        }]
                    }]
                }
            }
        },
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": server["wan"],
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.100.1",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    }
                }, {
                    "name": server["data"],
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.0.1",
                            "prefix-length": 24
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
                    "wireguard": {
                        "private-key": "server-wg-key",
                        "peers": [{
                            "public-key-bag": "roadwarriors",
                            "persistent-keepalive": 3,
                            "peer": [{
                                "public-key": "client1-key",
                                "allowed-ips": ["10.0.0.2/32", "192.168.1.0/24"]
                            }, {
                                "public-key": "client2-key",
                                "allowed-ips": ["10.0.0.3/32", "192.168.2.0/24"]
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
                                }, {
                                    "destination-prefix": "192.168.2.0/24",
                                    "next-hop": {
                                        "next-hop-address": "10.0.0.3"
                                    }
                                }, {
                                    "destination-prefix": "192.168.50.0/24",
                                    "next-hop": {
                                        "next-hop-address": "192.168.100.2"
                                    }
                                }, {
                                    "destination-prefix": "192.168.51.0/24",
                                    "next-hop": {
                                        "next-hop-address": "192.168.100.2"
                                    }
                                }]
                            }
                        }
                    }]
                }
            }
        }
    })

def configure_router(router):
    """Configure router to forward traffic between server and clients"""
    router.put_config_dicts({
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": router["wan"],
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.100.2",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    }
                }, {
                    "name": router["lan1"],
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.50.1",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    }
                }, {
                    "name": router["lan2"],
                    "ipv4": {
                        "address": [{
                            "ip": "192.168.51.1",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    }
                }]
            }
        }
    })

def configure_client(client, client_num, private_key, public_key, server_pubkey):
    """Configure a WireGuard roadwarrior client"""
    # Client1 is on 192.168.50.0/24, Client2 is on 192.168.51.0/24
    link_subnet = 49 + client_num

    client.put_config_dicts({
        "ietf-keystore": {
            "keystore": {
                "asymmetric-keys": {
                    "asymmetric-key": [{
                        "name": f"client{client_num}-wg-key",
                        "public-key-format": "infix-crypto-types:x25519-public-key-format",
                        "public-key": public_key,
                        "private-key-format": "infix-crypto-types:x25519-private-key-format",
                        "cleartext-private-key": private_key
                    }]
                }
            }
        },
        "ietf-truststore": {
            "truststore": {
                "public-key-bags": {
                    "public-key-bag": [{
                        "name": "server",
                        "public-key": [{
                            "name": "server-key",
                            "public-key-format": "infix-crypto-types:x25519-public-key-format",
                            "public-key": server_pubkey
                        }]
                    }]
                }
            }
        },
        "ietf-interfaces": {
            "interfaces": {
                "interface": [{
                    "name": client["link"],
                    "ipv4": {
                        "address": [{
                            "ip": f"192.168.{link_subnet}.2",
                            "prefix-length": 24
                        }]
                    }
                }, {
                    "name": client["data"],
                    "ipv4": {
                        "address": [{
                            "ip": f"192.168.{client_num}.1",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    }
                }, {
                    "name": "wg0",
                    "type": "infix-if-type:wireguard",
                    "ipv4": {
                        "address": [{
                            "ip": f"10.0.0.{client_num + 1}",
                            "prefix-length": 24
                        }],
                        "forwarding": True
                    },
                    "wireguard": {
                        "private-key": f"client{client_num}-wg-key",
                        "peers": [{
                            "public-key-bag": "server",
                            "endpoint": "192.168.100.1",
                            "endpoint-port": 51820,
                            "allowed-ips": ["10.0.0.0/24", "192.168.0.0/24"],
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
                                    "destination-prefix": "192.168.0.0/24",
                                    "next-hop": {
                                        "next-hop-address": "10.0.0.1"
                                    }
                                }, {
                                    "destination-prefix": "0.0.0.0/0",
                                    "next-hop": {
                                        "next-hop-address": f"192.168.{link_subnet}.1"
                                    }
                                }]
                            }
                        }
                    }]
                }
            }
        }
    })
server_private_key = "uIUL4AnD5QaVrwHDPHJzQ7sIQ+Q3zDdflnvfd59qa28="
server_public_key = "qGVmu5UbNtMuZs2t9wFoOoHlvgmV+A1SyQacVb/bEV0="

client1_private_key = "kNmkNlSkSh9+Va2tmFv9Va8TBCZlTBF0fKAGJf8vomo="
client1_public_key = "ROaZyvJc5DzA2XUAAeTj2YlwDsy2w0lr3t+rWj2imAk="

client2_private_key = "OPT7v/l5zICEmFIrO0U+YwA+w07l8Xo2Dp38hjGOHGY="
client2_public_key = "Om9CPLYdK3l93GauKrq5WXo/gbcD+1CeqFpobRLLkB4="


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUTs"):
        env = infamy.Env()
        server = env.attach("server", "mgmt")
        router = env.attach("router", "mgmt")
        client1 = env.attach("client1", "mgmt")
        client2 = env.attach("client2", "mgmt")

    _, hport_server = env.ltop.xlate("host", "data")
    _, hport_client1 = env.ltop.xlate("host", "data1")
    _, hport_client2 = env.ltop.xlate("host", "data2")

    with test.step("Configure DUTs"):
        util.parallel(
            lambda: configure_server(server, server_public_key, server_private_key, client1_public_key, client2_public_key),
            lambda: configure_router(router),
            lambda: configure_client(client1, 1, client1_private_key, client1_public_key, server_public_key),
            lambda: configure_client(client2, 2, client2_private_key, client2_public_key, server_public_key)
        )

    with test.step("Check on the server that both clients is connected"):
        util.parallel(lambda: util.until(lambda: wg.is_peer_up(server, "wg0", client1_public_key)),
                      lambda: util.until(lambda: wg.is_peer_up(server, "wg0", client2_public_key)))

    with infamy.IsolatedMacVlan(hport_server) as ns_server, \
         infamy.IsolatedMacVlan(hport_client1) as ns_client1, \
         infamy.IsolatedMacVlan(hport_client2) as ns_client2:
        ns_server.addip("192.168.0.2")
        ns_server.addroute("default", "192.168.0.1")

        ns_client1.addip("192.168.1.2")
        ns_client1.addroute("default", "192.168.1.1")

        ns_client2.addip("192.168.2.2")
        ns_client2.addroute("default", "192.168.2.1")

        with test.step("Verify IPv4 connectivity with ping 192.168.0.2 from host:data1 and host:data2"):
            util.parallel(
                lambda: ns_client1.must_reach("192.168.0.2"),
                lambda: ns_client2.must_reach("192.168.0.2")
            )

    test.succeed()
