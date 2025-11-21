#!/usr/bin/env python3
"""DHCPv6 Prefix Delegation

Verify DHCPv6 prefix delegation (IA_PD) where a client requests an IPv6
prefix from a DHCPv6 server.  This is commonly used on WAN interfaces of
routers to obtain a prefix for distribution to downstream networks.

"""

import infamy, infamy.dhcp
import infamy.iface as iface
from infamy.util import until
import time


def checkrun(dut):
    """Check DUT is running DHCPv6 client"""
    res = dut.runsh(f"pgrep -f 'udhcpc6.*{port}'")
    # print(f"Checking for udhcpc6: {res.stdout}")
    if res.stdout.strip() != "":
        return True
    return False


def checklog(dut):
    """Check syslog for prefix delegation message"""
    rc = dut.runsh("tail -10 /log/syslog | grep 'received delegated prefix'")
    # print(f"DHCPv6 client logs:\n{res.stdout}")
    if rc.stdout.strip() != "":
        return True
    return False


with infamy.Test() as test:
    SERVER = '2001:db8::1'
    CLIENT = '2001:db8::42'
    PREFIX = '2001:db8:1::/48'

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        client = env.attach("client", "mgmt")
        tgtssh = env.attach("client", "mgmt", "ssh")
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
                client.put_config_dict("ietf-interfaces", config)

            with test.step("Verify DHCPv6 client is running"):
                until(lambda: checkrun(tgtssh))

            with test.step("Verify prefix delegation in logs"):
                until(lambda: checklog(tgtssh))

    test.succeed()
