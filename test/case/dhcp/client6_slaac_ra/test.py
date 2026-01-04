#!/usr/bin/env python3
"""DHCPv6 with SLAAC/RA (Stateless DHCPv6)

Verify DHCPv6 client works in stateless mode (information-only) where:
- Router Advertisements (RA) provide the IPv6 address via SLAAC
- DHCPv6 provides DNS servers and domain search options only

This is a common ISP deployment scenario where the router sends RAs for
address autoconfiguration, and DHCPv6 is only used for providing additional
configuration like DNS servers.

The test verifies that odhcp6c correctly integrates both RA and DHCPv6
information, which is something the old udhcpc6 client could not do.

"""

import infamy
import infamy.dhcp
import infamy.iface as iface
from infamy.util import until


def checkrun(dut):
    """Check DUT is running DHCPv6 client"""
    rc = dut.runsh(f"pgrep -f 'odhcp6c.*{port}'")
    if rc.stdout.strip() != "":
        return True
    return False


def check_dns_resolution():
    """Check if DNS resolution works by pinging FQDN"""
    rc = tgtssh.runsh(f"ping -6 -c1 -w5 {VERIFY}")
    return rc.returncode == 0


def check_slaac_address():
    """Check if SLAAC address was assigned"""
    addrs = iface.get_ipv6_address(client, port)
    return addrs is not None and len(addrs) > 0


with infamy.Test() as test:
    SERVER = '2001:db8::1'
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
        # Stateless DHCPv6: RA provides address, DHCPv6 provides DNS
        with infamy.dhcp.Server6Dnsmasq(netns,
                                        start=None,      # No DHCPv6 addresses
                                        end=None,        # Stateless mode
                                        dns=SERVER,
                                        domain=DOMAIN,
                                        address=SERVER):

            with test.step("Configure DHCPv6 client in information-only mode"):
                config = {
                    "interfaces": {
                        "interface": [{
                            "name": f"{port}",
                            "ipv6": {
                                "enabled": True,
                                "infix-dhcpv6-client:dhcp": {
                                    "information-only": True,  # Stateless DHCPv6
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

            with test.step("Verify DHCPv6 client is running"):
                until(lambda: checkrun(tgtssh), attempts=20)

            with test.step("Verify client got SLAAC address from RA"):
                # Should get address from RA (SLAAC), not DHCPv6
                # Address will be in 2001:db8::/64 range
                until(check_slaac_address, attempts=30)

            with test.step("Verify client domain name resolution"):
                # This proves DNS came from DHCPv6 (information-only)
                until(check_dns_resolution, attempts=30)

    test.succeed()
