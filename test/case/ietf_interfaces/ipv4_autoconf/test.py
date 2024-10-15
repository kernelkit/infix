#!/usr/bin/env python3
"""
IPv4 link-local

Verifies that link-local (IPv4LL/ZeroConf) address assignment work as
expected.  Checks random address, the request-address setting, and
address removal on autoconf disable.
"""

import ipaddress
import infamy
import infamy.iface

from infamy.util import until


def has_linklocal(target, iface, request=None):
    """Check if interface as a link-local address"""
    addrs = infamy.iface.get_ipv4_address(target, iface)
    if not addrs:
        return False
    for addr in addrs:
        if addr['origin'] == "random":
            if request is None:
                print(f"Got IP address {addr['ip']}")
                return True
            if addr['ip'] == request:
                print(f"Got requested IP address {addr['ip']}")
                return True
    return False


def no_linklocal(target, iface):
    """Check if interface has no link-local address in the 169.254/16 range"""
    addrs = infamy.iface.get_ipv4_address(target, iface)
    if not addrs:
        return True
    for addr in addrs:
        ip = ipaddress.IPv4Address(addr['ip'])
        if ip in ipaddress.IPv4Network('169.254.0.0/16'):
            return False
    return True


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Configure interface target:mgmt with IPv4 ZeroConf IP"):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": tport,
                        "enabled": True,
                        "ipv4": {
                            "autoconf": {
                                "enabled": True
                            }
                        }
                    }
                ]
            }
        })

    with test.step("Verify link-local address exist on target:mgmt"):
        until(lambda: has_linklocal(target, tport), attempts=30)

    with test.step("Configure target:mgmt with a specific IPv4 ZeroConf IP"):
        _, tport = env.ltop.xlate("target", "data")

        target.put_config_dict("ietf-interfaces", {
            "interfaces": {
                "interface": [
                    {
                        "name": tport,
                        "enabled": True,
                        "ipv4": {
                            "autoconf": {
                                "enabled": True,
                                "request-address": "169.254.42.42"
                            }
                        }
                    }
                ]
            }
        })

    with test.step("Verify target:mgmt has link-local address 169.254.42.42"):
        until(lambda: has_linklocal(target, tport, request="169.254.42.42"),
              attempts=30)

    with test.step("Remove IPv4 link-local addresses from target:mgmt"):
        xpath = f"/ietf-interfaces:interfaces/interface[name='{tport}']" \
            "/ietf-ip:ipv4/infix-ip:autoconf"
        target.delete_xpath(xpath)

    with test.step("Verify link-local addresses has been removed from target:mgmt"):
        until(lambda: no_linklocal(target, tport), attempts=30)

    test.succeed()
