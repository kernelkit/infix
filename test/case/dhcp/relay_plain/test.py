#!/usr/bin/env python3
"""DHCP Relay without Option 82

Verify that the relay agent relays requests from an interface that is
not a bridge, and that option 82 can be turned off for one interface
while it is enabled for the relay agent.  The server has a static host
that matches the default circuit-id of the interface, its name, and a
pool of one other address.

The client first gets the address of the static host.  With option 82
turned off for the interface, a new client, here the same one with
another MAC address, gets the pool address.

"""
import infamy
import infamy.iface as iface
from infamy.util import parallel, until


with infamy.Test() as test:
    POOL    = '192.168.1.100'
    ADDRESS = '192.168.1.42'
    NEWMAC  = '02:00:00:00:00:99'

    with test.step("Set up topology and attach to client, relay and server DUTs"):
        env = infamy.Env()
        client, relay, server = parallel(lambda: env.attach("client", "mgmt"),
                                         lambda: env.attach("relay", "mgmt"),
                                         lambda: env.attach("server", "mgmt"))

    with test.step("Configure DHCP server with pool 192.168.1.100 and static host 192.168.1.42 for the relay interface"):
        server.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": server["link"],
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.2.2",
                                "prefix-length": 24
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
                                            "next-hop-address": "192.168.2.1"
                                        }
                                    }]
                                }
                            }
                        }]
                    }
                }
            },
            "infix-dhcp-server": {
                "dhcp-server": {
                    "subnet": [{
                        "subnet": "192.168.1.0/24",
                        "pool": {
                            "start-address": POOL,
                            "end-address":   POOL
                        },
                        "host": [{
                            "address": ADDRESS,
                            "match": {
                                "option82": {
                                    "circuit-id": {"str": relay["link1"]}
                                }
                            }
                        }]
                    }]
                }
            }})

    with test.step("Configure DHCP relay with option 82 on the client interface"):
        relay.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": relay["link1"],
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.1.1",
                                "prefix-length": 24
                            }]
                        }
                    }, {
                        "name": relay["link2"],
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.2.1",
                                "prefix-length": 24
                            }]
                        }
                    }]
                }
            },
            "infix-dhcp-relay": {
                "dhcp-relay": {
                    "server": [{"address": "192.168.2.2"}],
                    "option82": {},
                    "interface": [{
                        "name": relay["link1"]
                    }]
                }
            }})

    with test.step("Enable DHCP client on client"):
        client.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": client["link"],
                        "ipv4": {
                            "infix-dhcp-client:dhcp": {}
                        }
                    }]
                }
            }})

    with test.step("Verify client gets the address of the static host, 192.168.1.42"):
        until(lambda: iface.address_exist(client, client["link"], ADDRESS))

    with test.step("Disable option 82 for the client interface on relay"):
        relay.put_config_dicts({
            "infix-dhcp-relay": {
                "dhcp-relay": {
                    "interface": [{
                        "name": relay["link1"],
                        "option82": False
                    }]
                }
            }})

    with test.step("Change MAC address of client to 02:00:00:00:00:99"):
        client.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": client["link"],
                        "infix-interfaces:custom-phys-address": {
                            "static": NEWMAC
                        }
                    }]
                }
            }})

    with test.step("Verify client gets the pool address, 192.168.1.100"):
        until(lambda: iface.address_exist(client, client["link"], POOL))

    test.succeed()
