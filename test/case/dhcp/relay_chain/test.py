#!/usr/bin/env python3
"""DHCP Relay Chain

Verify relay agents in a chain of two switches, with the DHCP server
at the end of the chain and everything in the same subnet.  Both
switches add option 82, so the server can hand out an address per
switch and port:

 - sw1 gets its own address with DHCP.  Its request has no option 82
   when it arrives on the trusted port of sw2, which adds its own.
 - The client is connected to sw1, which relays its request to the
   server through sw2.  The replies from the server arrive on a port
   that sw1 relays from.

The server has a pool of one other address, which sw1 or the client
gets if option 82 is missing or wrong.

"""
import infamy
import infamy.iface as iface
from infamy.util import parallel, until


def bridge(dut, address=None):
    """Bridge br0 with ports down and up, with a static address or DHCP"""
    if address:
        ipv4 = {"address": [{"ip": address, "prefix-length": 24}]}
    else:
        ipv4 = {"infix-dhcp-client:dhcp": {}}

    return {
        "interfaces": {
            "interface": [{
                "name": "br0",
                "type": "infix-if-type:bridge",
                "ipv4": ipv4
            }, {
                "name": dut["down"],
                "infix-interfaces:bridge-port": {"bridge": "br0"}
            }, {
                "name": dut["up"],
                "infix-interfaces:bridge-port": {"bridge": "br0"}
            }]
        }
    }


def relay(server, remote_id, ports):
    """Relay from br0 to server, with settings for some ports"""
    return {
        "dhcp-relay": {
            "server": [{"address": server}],
            "option82": {
                "remote-id": {"str": remote_id}
            },
            "interface": [{
                "name": "br0",
                "port": ports
            }]
        }
    }


def host(address, remote_id, circuit_id):
    """Static host for a port of a switch"""
    return {
        "address": address,
        "match": {
            "option82": {
                "circuit-id": {"str": circuit_id},
                "remote-id":  {"str": remote_id}
            }
        }
    }


with infamy.Test() as test:
    SERVER  = '192.168.1.1'
    SW2     = '192.168.1.2'
    SW1     = '192.168.1.3'
    CLIENT  = '192.168.1.42'
    POOL    = '192.168.1.100'

    with test.step("Set up topology and attach to client, sw1, sw2 and server DUTs"):
        env = infamy.Env()
        client, sw1, sw2, server = parallel(lambda: env.attach("client", "mgmt"),
                                            lambda: env.attach("sw1", "mgmt"),
                                            lambda: env.attach("sw2", "mgmt"),
                                            lambda: env.attach("server", "mgmt"))

    with test.step("Configure DHCP server with static hosts 192.168.1.3 for sw1 and 192.168.1.42 for the client"):
        server.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": server["link"],
                        "ipv4": {
                            "address": [{
                                "ip": SERVER,
                                "prefix-length": 24
                            }]
                        }
                    }]
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
                        "host": [
                            host(SW1, "sw2", sw2["down"]),
                            host(CLIENT, "sw1", sw1["down"]),
                        ]
                    }]
                }
            }})

    with test.step("Configure sw2 with address 192.168.1.2 and DHCP relay, port to sw1 trusted"):
        sw2.put_config_dicts({
            "ietf-interfaces": bridge(sw2, SW2),
            "infix-dhcp-relay": relay(SERVER, "sw2", [
                {"name": sw2["down"], "trusted": True},
                {"name": sw2["up"], "enabled": False},
            ])})

    with test.step("Configure sw1 with DHCP client and DHCP relay, port to sw2 trusted"):
        sw1.put_config_dicts({
            "ietf-interfaces": bridge(sw1),
            "infix-dhcp-relay": relay(SERVER, "sw1", [
                {"name": sw1["up"], "trusted": True},
            ])})

    with test.step("Verify sw1 gets the address for its port on sw2, 192.168.1.3"):
        until(lambda: iface.address_exist(sw1, "br0", SW1))

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

    with test.step("Verify client gets the address for its port on sw1, 192.168.1.42"):
        until(lambda: iface.address_exist(client, client["link"], CLIENT))

    test.succeed()
