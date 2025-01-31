#!/usr/bin/env python3
"""DHCP Server Basic

Verify basic DHCP server functionality.  The server is set up to only
hand out leases from a pool of a single address.  A single DHCP option,
hostname, is to be handed out with the lease to the client.  Ensure no
other options are sent by checking, e.g., that the client has not set up
a default route to the server.

"""
import infamy
import infamy.iface as iface
import infamy.route as route
from infamy.util import until


def verify_hostname(dut, hostname):
    """Verify dut hostname"""
    oper = dut.get_data("/ietf-system:system")
    curr = oper["system"]["hostname"]
    # print(f"Hostname now: {curr}")
    if curr == hostname:
        return True
    return False


with infamy.Test() as test:
    ADDRESS = '192.168.2.100'
    HOSTNM1 = 'foo'
    HOSTNM2 = 'client'
    with test.step("Set up topology and attach to client and server DUTs"):
        env = infamy.Env()
        client = env.attach("client", "mgmt")
        server = env.attach("server", "mgmt")

    with test.step("Configure DHCP server and client"):
        server.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": server["link"],
                        "ipv4": {
                            "address": [{
                                "ip": "192.168.2.1",
                                "prefix-length": 24
                            }]
                        }
                    }]
                }
            },
            "infix-dhcp-server": {
                "dhcp-server": {
                    "subnet": [{
                        "subnet": "192.168.2.0/24",
                        "option": [{
                            "id": "hostname",
                            "name": HOSTNM2
                        }],
                        "pool": {
                            "start-address": ADDRESS,
                            "end-address":   ADDRESS,
                        }
                    }]
                }
            },
            "ietf-system": {
                "system": {"hostname": "server.example.com"}
            }})

        client.put_config_dicts({
            "infix-dhcp-client": {
                "dhcp-client": {
                    "client-if": [{
                        "if-name": client["link"],
                        "option": [
                            {"id": "hostname"},
                            {"id": "domain"}
                        ]
                    }]
                }
            },
            "ietf-system": {
                "system": {"hostname": HOSTNM1}
            }})

    with test.step("Verify DHCP client's original hostname"):
        until(lambda: verify_hostname(client, HOSTNM1))

    with test.step("Verify DHCP client lease from server's pool"):
        until(lambda: iface.address_exist(client, client["link"], ADDRESS))

    with test.step("Verify DHCP client's new hostname"):
        until(lambda: verify_hostname(client, HOSTNM2))

    with test.step("Verify DHCP client has no default route"):
        until(lambda: route.ipv4_route_exist(client, "0.0.0.0/0") is False)

    test.succeed()
