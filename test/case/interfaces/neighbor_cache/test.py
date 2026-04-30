#!/usr/bin/env python3
"""
ARP and Neighbor Cache

Verify that static ARP entries (IPv4) and neighbor cache entries (IPv6)
can be configured on an interface and are immediately visible in the
operational datastore with origin "static".  Also verify that removing
the entries causes them to disappear from the operational datastore.
"""
import infamy
import infamy.iface as iface

from infamy.util import until

IPV4_NEIGH = "192.0.2.1"
IPV4_LLADR = "de:ad:be:ef:ca:fe"
IPV6_NEIGH = "2001:db8::1"
IPV6_LLADR = "de:ad:be:ef:ca:ff"

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        _, tport = env.ltop.xlate("target", "data")

    with test.step("Configure static IPv4 ARP and IPv6 neighbor entries on target:data"):
        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [{
                    "name": tport,
                    "ipv4": {
                        "neighbor": [{
                            "ip": IPV4_NEIGH,
                            "link-layer-address": IPV4_LLADR,
                        }]
                    },
                    "ipv6": {
                        "neighbor": [{
                            "ip": IPV6_NEIGH,
                            "link-layer-address": IPV6_LLADR,
                        }]
                    }
                }]
            }
        })

    with test.step("Verify static IPv4 ARP entry is visible in operational state"):
        until(lambda: iface.neighbor_exist(target, tport, IPV4_NEIGH, IPV4_LLADR, "static"))

    with test.step("Verify static IPv6 neighbor entry is visible in operational state"):
        until(lambda: iface.neighbor_exist(target, tport, IPV6_NEIGH, IPV6_LLADR, "static"))

    with test.step("Remove static neighbor entries by clearing IPv4 and IPv6 config"):
        target.delete_xpath(
            f"/ietf-interfaces:interfaces/interface[name='{tport}']/ietf-ip:ipv4")
        target.delete_xpath(
            f"/ietf-interfaces:interfaces/interface[name='{tport}']/ietf-ip:ipv6")

    with test.step("Verify static neighbor entries are no longer present"):
        until(lambda: not iface.neighbor_exist(target, tport, IPV4_NEIGH))
        until(lambda: not iface.neighbor_exist(target, tport, IPV6_NEIGH))

    test.succeed()
