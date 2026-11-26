#!/usr/bin/env python3
"""DHCP Relay with Local Server

Verify that a device can hand out an address per port: the relay agent
adds option 82 to requests from a bridge port and relays them to the
DHCP server on the same device, which has a static host matching the
circuit-id of the port, by default the name of the port.

The static host has preempt enabled.  When the client is replaced, here
by changing the MAC address of the client, the new one gets the same
address at once, even though the lease of the old one has not expired.

"""
import infamy
import infamy.iface as iface
from infamy.util import parallel, until


def lease_mac(server, address):
    """MAC address of the client holding the lease for address"""
    data = server.get_data("/infix-dhcp-server:dhcp-server/leases")
    if not data:
        return None
    for lease in data["dhcp-server"].get("leases", {}).get("lease", []):
        if lease["address"] == address:
            return lease["phys-address"]
    return None


with infamy.Test() as test:
    POOL    = '192.168.1.100'
    ADDRESS = '192.168.1.42'
    NEWMAC  = '02:00:00:00:00:99'

    with test.step("Set up topology and attach to client and relay DUTs"):
        env = infamy.Env()
        client, relay = parallel(lambda: env.attach("client", "mgmt"),
                                 lambda: env.attach("relay", "mgmt"))

    with test.step("Configure relay with br0, DHCP relay to 127.0.0.1 and static host 192.168.1.42 for the port"):
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
                        "name": relay["link"],
                        "infix-interfaces:bridge-port": {
                            "bridge": "br0"
                        }
                    }]
                }
            },
            "infix-dhcp-relay": {
                "dhcp-relay": {
                    "server": [{"address": "127.0.0.1"}],
                    "option82": {},
                    "interface": [{"name": "br0"}]
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
                                    "circuit-id": {"str": relay["link"]}
                                }
                            },
                            "preempt": True
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

    with test.step("Verify client gets the address of the port, 192.168.1.42"):
        until(lambda: iface.address_exist(client, client["link"], ADDRESS))

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

    with test.step("Verify lease for 192.168.1.42 is held by 02:00:00:00:00:99"):
        until(lambda: lease_mac(relay, ADDRESS) == NEWMAC)

    test.succeed()
