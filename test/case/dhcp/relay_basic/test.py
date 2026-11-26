#!/usr/bin/env python3
"""DHCP Relay Basic

Verify that the relay agent relays requests from a bridge port to a
DHCP server on another subnet, and adds option 82 to them.  The port
has a configured circuit-id, and the relay agent a remote-id.  The
server has a static host that matches both, and a pool of one other
address, which the client gets if option 82 is missing or wrong.

"""
import infamy
import infamy.iface as iface
from infamy.util import parallel, until


with infamy.Test() as test:
    POOL    = '192.168.1.100'
    ADDRESS = '192.168.1.42'
    CIRCUIT = 'camera-18'
    REMOTE  = 'switch-17'

    with test.step("Set up topology and attach to client, relay and server DUTs"):
        env = infamy.Env()
        client, relay, server = parallel(lambda: env.attach("client", "mgmt"),
                                         lambda: env.attach("relay", "mgmt"),
                                         lambda: env.attach("server", "mgmt"))

    with test.step("Configure DHCP server with static host for circuit-id camera-18"):
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
                                    "circuit-id": {"str": CIRCUIT},
                                    "remote-id":  {"str": REMOTE}
                                }
                            }
                        }]
                    }]
                }
            }})

    with test.step("Configure bridge and DHCP relay with circuit-id camera-18"):
        relay.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": "br0",
                        "type": "infix-if-type:bridge",
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.1.1",
                                "prefix-length": 24
                            }]
                        }
                    }, {
                        "name": relay["link1"],
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
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
                    "option82": {
                        "remote-id": {"str": REMOTE}
                    },
                    "interface": [{
                        "name": "br0",
                        "port": [{
                            "name": relay["link1"],
                            "circuit-id": {"str": CIRCUIT}
                        }]
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

    with test.step("Verify client gets the address for camera-18, 192.168.1.42"):
        until(lambda: iface.address_exist(client, client["link"], ADDRESS))

    test.succeed()
