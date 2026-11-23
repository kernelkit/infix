#!/usr/bin/env python3
"""DHCP Relay with VLANs

Verify the relay agent on a VLAN filtering bridge, with the DHCP server
on the same device and a subnet per VLAN.  Port link1 is an access port
in VLAN 10 and port link2 a trunk port with VLAN 10 and 20.  The server
has a static host per port and VLAN:

 - client1, untagged on the access port, gets 192.168.10.42
 - client2, tagged with VLAN 10 on the trunk port, gets 192.168.10.43
 - client2, tagged with VLAN 20 on the trunk port, gets 192.168.20.43

"""
import infamy
import infamy.iface as iface
from infamy.util import parallel, until


def vlan(name, vid, lower, ipv4):
    """VLAN interface on top of lower"""
    return {
        "name": name,
        "type": "infix-if-type:vlan",
        "vlan": {
            "id": vid,
            "lower-layer-if": lower
        },
        "ipv4": ipv4
    }


def address(ip):
    """Static IPv4 address"""
    return {"address": [{"ip": ip, "prefix-length": 24}]}


def subnet(net, hosts):
    """DHCP subnet with static hosts, address by circuit-id"""
    return {
        "subnet": net,
        "host": [{
            "address": addr,
            "match": {
                "option82": {
                    "circuit-id": {"str": port}
                }
            }
        } for addr, port in hosts]
    }


with infamy.Test() as test:
    ACCESS10 = '192.168.10.42'
    TRUNK10  = '192.168.10.43'
    TRUNK20  = '192.168.20.43'
    DHCP     = {"infix-dhcp-client:dhcp": {}}

    with test.step("Set up topology and attach to relay, client1 and client2 DUTs"):
        env = infamy.Env()
        relay, client1, client2 = parallel(lambda: env.attach("relay", "mgmt"),
                                           lambda: env.attach("client1", "mgmt"),
                                           lambda: env.attach("client2", "mgmt"))

    with test.step("Configure relay with bridge br0, VLAN 10 and 20, access port link1 and trunk port link2"):
        access = relay["link1"]
        trunk = relay["link2"]
        relay.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": "br0",
                            "type": "infix-if-type:bridge",
                            "bridge": {
                                "vlans": {
                                    "vlan": [{
                                        "vid": 10,
                                        "untagged": [access],
                                        "tagged":   [trunk, "br0"]
                                    }, {
                                        "vid": 20,
                                        "tagged":   [trunk, "br0"]
                                    }]
                                }
                            }
                        }, {
                            "name": access,
                            "infix-interfaces:bridge-port": {
                                "pvid": 10,
                                "bridge": "br0"
                            }
                        }, {
                            "name": trunk,
                            "infix-interfaces:bridge-port": {
                                "bridge": "br0"
                            }
                        },
                        vlan("vlan10", 10, "br0", address("192.168.10.1")),
                        vlan("vlan20", 20, "br0", address("192.168.20.1")),
                    ]
                }
            }})

    with test.step("Configure DHCP relay on vlan10 and vlan20, and DHCP server with a static host per port and VLAN"):
        relay.put_config_dicts({
            "infix-dhcp-relay": {
                "dhcp-relay": {
                    "server": [{"address": "127.0.0.1"}],
                    "option82": {},
                    "interface": [
                        {"name": "vlan10"},
                        {"name": "vlan20"},
                    ]
                }
            },
            "infix-dhcp-server": {
                "dhcp-server": {
                    "subnet": [
                        subnet("192.168.10.0/24", [(ACCESS10, access), (TRUNK10, trunk)]),
                        subnet("192.168.20.0/24", [(TRUNK20, trunk)]),
                    ]
                }
            }})

    with test.step("Enable DHCP client on client1, and on VLAN 10 and 20 of client2"):
        link = client2["link"]
        parallel(
            lambda: client1.put_config_dicts({
                "ietf-interfaces": {
                    "interfaces": {
                        "interface": [{
                            "name": client1["link"],
                            "ipv4": DHCP
                        }]
                    }
                }}),
            lambda: client2.put_config_dicts({
                "ietf-interfaces": {
                    "interfaces": {
                        "interface": [
                            vlan(f"{link}.10", 10, link, DHCP),
                            vlan(f"{link}.20", 20, link, DHCP),
                        ]
                    }
                }}),
        )

    with test.step("Verify client1 gets 192.168.10.42"):
        until(lambda: iface.address_exist(client1, client1["link"], ACCESS10))

    with test.step("Verify client2 gets 192.168.10.43 on VLAN 10"):
        until(lambda: iface.address_exist(client2, f"{link}.10", TRUNK10))

    with test.step("Verify client2 gets 192.168.20.43 on VLAN 20"):
        until(lambda: iface.address_exist(client2, f"{link}.20", TRUNK20))

    test.succeed()
