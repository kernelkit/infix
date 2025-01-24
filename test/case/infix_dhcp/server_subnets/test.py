#!/usr/bin/env python3
"""DHCP Server Multiple Subnets

Verify that the DHCP server is capble of acting on more than one subnet,
handing out leases from a pool and static host lease, ensuring global,
subnet, and host-specific options are honored and do not leak between
subnets.

.Internal network setup, client2 and client3 are on the same LAN
image::dhcp-subnets.svg[Internal networks]

To verify isolation of subnet settings, a few "decoys" are added to the
configuration of each subnet.  These are then checked for on each of the
clients.  E.g., both subnets have static host configurations, but only
one client should match.

Both DNS and NTP servers are handed out to clients. Some clients have
a static DNS and NTP server configured already.

The test is concluded by the server trying to reach each client using
ping of the hostname.

"""
import infamy
import infamy.iface as iface
import infamy.route as route
from infamy.util import until


def has_dns_server(data, dns_servers):
    """Verify system have all the given DNS servers"""
    servers = data.get('system-state', {}) \
                  .get('dns-resolver', {}) \
                  .get('server')
    if not servers:
        return False

    configured = {server['address'] for server in servers}
    return all(server in configured for server in dns_servers)


def has_ntp_server(data, ntp_server):
    """Verify system has, or does *not* have*, an NTP server"""
    sources = data.get('system-state', {}) \
                  .get('ntp', {}) \
                  .get('sources', {}) \
                  .get('source')
    if not ntp_server:
        # This system should *not* have any NTP server
        return not sources

    if not sources:
        return False

    for source in sources:
        if source['address'] == ntp_server:
            return True

    return False


def has_system_servers(dut, dns, ntp=None):
    """Verify DUT have all DNS and, if given, NTP server(s)"""
    data = dut.get_data("/ietf-system:system-state")
    if data is None:
        return False

    return has_dns_server(data, dns) and has_ntp_server(data, ntp)


with infamy.Test() as test:
    SERVER1 = '192.168.1.1'
    POOL1   = '192.168.1.100'
    ADDR1   = '192.168.1.11'
    SERVER2 = '192.168.2.1'
    POOL2   = '192.168.2.200'
    ADDR2   = '192.168.2.22'
    GW1     = '192.168.1.11'
    GW2     = '192.168.2.2'
    HOSTNM1 = 'client1'
    HOSTNM2 = 'client2'
    HOSTNM3 = 'client3'

    with test.step("Set up topology and attach to client and server DUTs"):
        env = infamy.Env()
        server = env.attach("server", "mgmt")
        client1 = env.attach("client1", "mgmt")
        client2 = env.attach("client2", "mgmt")
        client3 = env.attach("client3", "mgmt")

    with test.step("Configure DHCP server and clients"):
        server.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [
                        {
                            "name": server["client1"],
                            "ipv4": {
                                "address": [{
                                    "ip": SERVER1,
                                    "prefix-length": 24
                                }]
                            }
                        }, {
                            "name": "br0",
                            "type": "infix-if-type:bridge",
                            "ipv4": {
                                "address": [{
                                    "ip": SERVER2,
                                    "prefix-length": 24,
                                }]
                            }
                        }, {
                            "name": server["client2"],
                            "infix-interfaces:bridge-port": {
                                "bridge": "br0"
                            }
                        }, {
                            "name": server["client3"],
                            "infix-interfaces:bridge-port": {
                                "bridge": "br0"
                            }
                        },
                    ]
                }
            },
            "infix-dhcp-server": {
                "dhcp-server": {
                    "option": [
                        {
                            "id": "dns-server",
                            "address": "auto"
                        },
                    ],
                    # No client should get the static host lease on this
                    # subnet.  Only a pool address and global options.
                    "subnet": [
                        {
                            "subnet": "192.168.1.0/24",
                            "option": [{
                                "id": "router",
                                "address": "auto"
                            }],
                            "pool": {
                                "start-address": POOL1,
                                "end-address":   POOL1
                            },
                            # Decoy, client2 should not get this lease!
                            "host": [{
                                "address": ADDR1,
                                "match": {
                                    "hostname": HOSTNM2
                                },
                                "option": [{
                                    "id": "classless-static-route",
                                    "static-route": [{
                                        "destination": "0.0.0.0/0",
                                        "next-hop": GW1
                                    }]
                                }]
                            }]
                        },
                        {
                            "subnet": "192.168.2.0/24",
                            "option": [
                                {
                                    "id": "ntp-server",
                                    "address": "auto"
                                }, {
                                    # Verify correct option selection in
                                    # client: option 3 < option 121
                                    "id": "router",
                                    "address": "auto"
                                }
                            ],
                            "pool": {
                                "start-address": POOL2,
                                "end-address":   POOL2
                            },
                            "host": [{
                                "address": ADDR2,
                                "match": {
                                    "hostname": HOSTNM2
                                },
                                "option": [{
                                    "id": "classless-static-route",
                                    "static-route": [{
                                        "destination": "0.0.0.0/0",
                                        "next-hop": GW2
                                    }]
                                }]
                            }]
                        },
                    ]
                }
            }})

        # All clients request/accept the same options.  We do this to
        # both keep fleet configuration simple but also to verify that
        # the server is behaving correctly.

        client1.put_config_dicts({
            "infix-dhcp-client": {
                "dhcp-client": {
                    "client-if": [{
                        "if-name": client1["server"],
                        "option": [
                            {"id": "hostname", "value": "auto"},
                            {"id": "router"},
                            {"id": "dns"},
                            {"id": "ntpsrv"},
                            {"id": 121}
                        ]
                    }]
                }
            },
            "ietf-system": {
                "system": {
                    "hostname": HOSTNM1,
                    "ntp": {"enabled": True},
                }
            }})

        client2.put_config_dicts({
            "infix-dhcp-client": {
                "dhcp-client": {
                    "client-if": [{
                        "if-name": client2["server"],
                        "option": [
                            {"id": "hostname", "value": "auto"},
                            {"id": "router"},
                            {"id": "dns"},
                            {"id": "ntpsrv"},
                            {"id": 121}
                        ]
                    }]
                }
            },
            "ietf-system": {
                "system": {
                    "hostname": HOSTNM2,
                    "ntp": {"enabled": True},
                }
            }})

        client3.put_config_dicts({
            "infix-dhcp-client": {
                "dhcp-client": {
                    "client-if": [{
                        "if-name": client3["server"],
                        "option": [
                            {"id": "hostname", "value": "auto"},
                            {"id": "router"},
                            {"id": "dns"},
                            {"id": "ntpsrv"},
                            {"id": 121}
                        ]
                    }]
                }
            },
            "ietf-system": {
                "system": {
                    "hostname": HOSTNM3,
                    "ntp": {"enabled": True},
                    "dns-resolver": {
                        "search": [
                          "example.com",
                          "kernelkit.org"
                        ],
                        "server": [
                          {
                            "name": "static",
                            "udp-and-tcp": {
                              "address": "1.2.3.4"
                            }
                          }
                        ],
                        "options": {
                          "timeout": 3,
                          "attempts": 5
                        }
                    },
                }
            }})

    with test.step("Verify DHCP client1 get correct lease"):
        until(lambda: iface.address_exist(client1, client1["server"], POOL1))

    with test.step("Verify DHCP client1 has default route via server"):
        until(lambda: route.ipv4_route_exist(client1, "0.0.0.0/0", nexthop=SERVER1))

    with test.step("Verify DHCP client1 has correct DNS server(s)"):
        until(lambda: has_system_servers(client1, ["192.168.1.1"]))

    with test.step("Verify DHCP client2 get correct static lease"):
        until(lambda: iface.address_exist(client2, client2["server"], ADDR2))

    with test.step("Verify DHCP client2 has default route via classless-static-route"):
        until(lambda: route.ipv4_route_exist(client2, "0.0.0.0/0", nexthop=GW2))

    with test.step("Verify DHCP client2 has correct DNS and NTP server(s)"):
        until(lambda: has_system_servers(client2, ["192.168.2.1"], "192.168.2.1"))

    with test.step("Verify DHCP client3 get correct lease"):
        until(lambda: iface.address_exist(client3, client3["server"], POOL2))

    with test.step("Verify DHCP client3 has default route via server"):
        until(lambda: route.ipv4_route_exist(client3, "0.0.0.0/0", nexthop=SERVER2))

    with test.step("Verify DHCP client3 has correct DNS and NTP server(s)"):
        until(lambda: has_system_servers(client3, ["1.2.3.4", "192.168.2.1"], "192.168.2.1"))

    test.succeed()
