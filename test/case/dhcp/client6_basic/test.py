#!/usr/bin/env python3
"""DHCPv6 Basic

Enable a DHCPv6 client and verify it requests an IPv6 lease from a
DHCPv6 server that is then set on the interface.

"""

import infamy
import infamy.dhcp
import infamy.iface as iface
import infamy.route as route
from   infamy.util import until

with infamy.Test() as test:
    SERVER = '2001:db8::1'
    CLIENT = '2001:db8::42'
    DOMAIN = 'example.com'
    VERIFY = 'server.' + DOMAIN

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        client = env.attach("client", "mgmt")
        tgtssh = env.attach("client", "mgmt", "ssh")
        _, host = env.ltop.xlate("host", "data")
        _, port = env.ltop.xlate("client", "data")

    with infamy.IsolatedMacVlan(host) as netns:
        netns.addip(SERVER, prefix_length=64, proto="ipv6")
        with infamy.dhcp.Server6Dnsmasq(netns,
                                        start=CLIENT,
                                        end=CLIENT,
                                        dns=SERVER,
                                        domain=DOMAIN,
                                        address=SERVER):

            with test.step("Configure DHCPv6 client"):
                config = {
                    "interfaces": {
                        "interface": [{
                            "name": f"{port}",
                            "ipv6": {
                                "enabled": True,
                                "infix-dhcpv6-client:dhcp": {
                                    "option": [
                                        {"id": "dns-server"},
                                        {"id": "domain-search"}
                                    ]
                                }
                            }
                        }]
                    }
                }
                client.put_config_dict("ietf-interfaces", config)

            with test.step(f"Verify client lease for {CLIENT}"):
                until(lambda: iface.address_exist(client, port, CLIENT, prefix_length=128))

            with test.step("Verify client default route ::/0"):
                until(lambda: route.ipv6_route_exist(client, "::/0"), attempts=20)

            with test.step("Verify client domain name resolution"):
                rc = tgtssh.runsh(f"ping -6 -c1 -w20 {VERIFY}")
                assert rc.returncode == 0, f"Client failed: ping {VERIFY}"

    test.succeed()
