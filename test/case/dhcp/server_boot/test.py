#!/usr/bin/env python3
"""DHCP Server Network Boot

Verify that network boot parameters are handed out in the BOOTP header
fields, which BOOTP clients, U-Boot and PXE ROMs read, and that the most
specific scope wins: host over subnet over global.

The DHCP client on the host records the server address (siaddr) and
boot file it receives in the lease.

"""
import infamy
import infamy.dhcp as dhcp
from infamy.util import until

SERVER = "10.0.0.10"
OTHER_SERVER = "10.0.0.20"
GLOBAL_FILE = "global.itb"
SUBNET_FILE = "subnet.itb"
HOST_FILE = "host.itb"
HOST_MAC = "02:00:00:00:be:ef"
HOST_ADDR = "10.0.0.50"
SUBNET = "10.0.0.0/24"

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, hport = env.ltop.xlate("host", "data")

    with test.step("Configure DHCP server with global, subnet, and host boot parameters"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": target["data"],
                        "enabled": True,
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
                    "boot": {
                        "file": GLOBAL_FILE,
                        "server-address": OTHER_SERVER
                    },
                    "subnet": [{
                        "subnet": SUBNET,
                        "boot": {
                            "file": SUBNET_FILE
                        },
                        "pool": {
                            "start-address": "10.0.0.100",
                            "end-address": "10.0.0.100"
                        },
                        "host": [{
                            "address": HOST_ADDR,
                            "match": {
                                "mac-address": HOST_MAC
                            },
                            "boot": {
                                "file": HOST_FILE
                            }
                        }]
                    }]
                }
            }
        })

    with infamy.IsolatedMacVlan(hport) as ns:
        client = dhcp.Client(ns)

        with test.step("Verify pool client gets subnet boot file from this server"):
            until(lambda: client.lease() == (SERVER, SUBNET_FILE))

        with test.step("Verify static host gets its own boot file"):
            ns.run(["ip", "link", "set", "iface", "address", HOST_MAC])
            until(lambda: client.lease() == (SERVER, HOST_FILE))

        with test.step("Remove subnet and host boot, verify client falls back to global boot file and server"):
            target.delete_xpath(f"/infix-dhcp-server:dhcp-server/subnet[subnet='{SUBNET}']/boot")
            target.delete_xpath(f"/infix-dhcp-server:dhcp-server/subnet[subnet='{SUBNET}']"
                                f"/host[address='{HOST_ADDR}']/boot")
            until(lambda: client.lease() == (OTHER_SERVER, GLOBAL_FILE))

    test.succeed()
