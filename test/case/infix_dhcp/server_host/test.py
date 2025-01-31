#!/usr/bin/env python3
"""DHCP Server Static Host

Verify DHCP server can hand out static host leases based on client-id,
both hexadecimal and a very long string, ensuring no pool address is
handed out instead.

"""
import infamy
import infamy.iface as iface
import infamy.route as route
from infamy.util import until


with infamy.Test() as test:
    POOL1    = '192.168.1.100'
    ADDRESS1 = '192.168.1.11'
    GW1      = '192.168.1.2'
    POOL2    = '192.168.2.100'
    ADDRESS2 = '192.168.2.22'
    GW2      = '192.168.2.1'
    HOSTNM1  = 'foo'
    HOSTCID1 = '00:c0:ff:ee'    # Infix DHCP server is RFC compliant
    HOSTNM11 = 'client1'
    HOSTCID2 = 'xyzzydissiegillespiefoobarterrawinklesouponastick'
    HOSTNM2  = 'bar'
    HOSTNM22 = 'client2'

    with test.step("Set up topology and attach to client and server DUTs"):
        env = infamy.Env()
        server = env.attach("server", "mgmt")
        client1 = env.attach("client1", "mgmt")
        client2 = env.attach("client2", "mgmt")

    with test.step("Configure DHCP client and server DUTs"):
        server.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": server["link1"],
                            "ipv4": {
                                "address": [{
                                    "ip": "192.168.1.1",
                                    "prefix-length": 24
                                }]
                            }
                        }, {
                            "name": server["link2"],
                            "ipv4": {
                                "address": [{
                                    "ip": "192.168.2.1",
                                    "prefix-length": 24
                                }]
                            }
                        },
                    ]
                }
            },
            "infix-dhcp-server": {
                "dhcp-server": {
                    "option": [{
                        "id": "router", "address": "auto"
                    }],
                    "subnet": [
                        {
                            "subnet": "192.168.1.0/24",
                            "pool": {
                                "start-address": POOL1,
                                "end-address":   POOL1
                            },
                            "host": [{
                                "address": ADDRESS1,
                                "match": {
                                    "client-id": {"hex": HOSTCID1}
                                },
                                "option": [
                                    {
                                        "id": "hostname",
                                        "name": HOSTNM11
                                    }, {
                                        "id": "classless-static-route",
                                        "static-route": [{
                                            "destination": "0.0.0.0/0",
                                            "next-hop": GW1
                                        }]
                                    }
                                ]
                            }]
                        }, {
                            "subnet": "192.168.2.0/24",
                            "pool": {
                                "start-address": POOL2,
                                "end-address":   POOL2
                            },
                            "host": [{
                                "address": ADDRESS2,
                                "match": {
                                    "client-id": {"str": HOSTCID2}
                                },
                                "option": [
                                    {
                                        "id": "hostname",
                                        "name": HOSTNM22
                                    }
                                ],
                                "lease-time": "infinite"
                            }]
                        },
                    ]
                }
            }})

        # We request hostname option just to ensure we don't get it.
        client1.put_config_dicts({
            "ietf-system": {
                "system": {"hostname": HOSTNM1}
            },
            "infix-dhcp-client": {
                "dhcp-client": {
                    "client-if": [{
                        "if-name": client1["link"],
                        "option": [
                            {"id": "router"},
                            {"id": "client-id", "hex": HOSTCID1},
                            {"id": 121}
                        ]
                    }]
                }
            },
        })

        client2.put_config_dicts({
            "ietf-system": {
                "system": {"hostname": HOSTNM2}
            },
            "infix-dhcp-client": {
                "dhcp-client": {
                    "client-if": [{
                        "if-name": client2["link"],
                        "client-id": HOSTCID2,
                        "option": [
                            {"id": "router"},
                            {"id": "hostname"},
                            {"id": 121}
                        ]
                    }]
                }
            },
        })

    with test.step("Verify DHCP client1 static lease"):
        until(lambda: iface.address_exist(client1, client1["link"], ADDRESS1))

    with test.step("Verify client1 hostname has *not* changed"):
        until(lambda: client1.get_data("/ietf-system:system")["system"]["hostname"] == HOSTNM1)

    with test.step("Verify DHCP client1 has default route via option 121"):
        until(lambda: route.ipv4_route_exist(client1, "0.0.0.0/0", nexthop=GW1))

    with test.step("Verify DHCP client2 static lease"):
        until(lambda: iface.address_exist(client2, client2["link"], ADDRESS2))

    with test.step("Verify client2 hostname has changed"):
        until(lambda: client2.get_data("/ietf-system:system")["system"]["hostname"] == HOSTNM22)

    with test.step("Verify DHCP client2 has default gateway"):
        until(lambda: route.ipv4_route_exist(client2, "0.0.0.0/0", nexthop=GW2))

    test.succeed()
