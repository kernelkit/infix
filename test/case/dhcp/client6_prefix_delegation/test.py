#!/usr/bin/env python3
"""DHCPv6 Prefix Delegation

Verify DHCPv6 prefix delegation (IA_PD) where a client requests an IPv6
prefix from a DHCPv6 server.  This is commonly used on WAN interfaces of
routers to obtain a prefix for distribution to downstream networks.

The client must install an unreachable route for the delegated prefix,
as a static route, to prevent routing loops.

"""

import infamy, infamy.dhcp
import infamy.iface as iface
import infamy.route as route
from infamy.util import parallel, until
import time


def checkrun(dut):
    """Check DUT is running DHCPv6 client"""
    res = dut.runsh(f"pgrep -f 'odhcp6c.*{port}'")
    # print(f"Checking for odhcp6c: {res.stdout}")
    if res.stdout.strip() != "":
        return True
    return False


def delegated_prefix(dut):
    """Delegated prefix from the client's log, or None"""
    rc = dut.runsh("tail -50 /log/syslog | grep -o 'received delegated prefix [^ ]*'")
    words = rc.stdout.split()
    return words[-1] if words else None


def checklog(dut):
    """Check syslog for prefix delegation message"""
    return delegated_prefix(dut) is not None


with infamy.Test() as test:
    SERVER = '2001:db8::1'
    CLIENT = '2001:db8::42'
    PREFIX = '2001:db8:1::/48'

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        client, tgtssh = parallel(lambda: env.attach("client", "mgmt"),
                                  lambda: env.attach("client", "mgmt", "ssh"))
        _, host = env.ltop.xlate("host", "data")
        _, port = env.ltop.xlate("client", "data")

    with infamy.IsolatedMacVlan(host) as netns:
        netns.addip(SERVER, prefix_length=48, proto="ipv6")
        with infamy.dhcp.Server6Dhcpd(netns=netns,
                                      start="2001:db8::100",
                                      end="2001:db8::200",
                                      prefix="2001:db8:100::",
                                      prefix_len=64,
                                      dns="2001:db8::1",
                                      iface="iface",
                                      subnet="2001:db8::/48"):

            with test.step("Configure DHCPv6 client w/ prefix delegation"):
                config = {
                    "interfaces": {
                        "interface": [{
                            "name": f"{port}",
                            "ipv6": {
                                "enabled": True,
                                "infix-dhcpv6-client:dhcp": {
                                    "option": [
                                        {"id": "dns-server"},
                                        {"id": "ia-pd"}
                                    ]
                                }
                            }
                        }]
                    }
                }
                client.put_config_dicts({"ietf-interfaces": config})

            with test.step("Verify DHCPv6 client is running"):
                until(lambda: checkrun(tgtssh), attempts=20)

            with test.step("Verify prefix delegation in logs"):
                # Prefix delegation may take longer on ARM hardware
                until(lambda: checklog(tgtssh), attempts=30)

            with test.step("Verify unreachable route for the delegated prefix"):
                pd = delegated_prefix(tgtssh)
                until(lambda: route.ipv6_route_exist(client, pd, proto="ietf-routing:static"),
                      attempts=30)

    test.succeed()
