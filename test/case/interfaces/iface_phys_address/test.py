#!/usr/bin/env python3
"""
Custom MAC address on interface

Verify support for setting and removing a custom MAC address on interfaces.
Both static MAC address and derived from the chassis MAC with, or without,
an offset applied.
"""

import infamy
import infamy.iface as iface
from infamy.util import until


def calc_mac(base_mac, mac_offset):
    """Add mac_offset to base_mac and return result."""
    base = [int(x, 16) for x in base_mac.split(':')]
    offset = [int(x, 16) for x in mac_offset.split(':')]
    result = [0] * 6
    carry = 0

    for i in range(5, -1, -1):
        total = base[i] + offset[i] + carry
        result[i] = total & 0xFF
        carry = 1 if total > 0xFF else 0

    return ':'.join(f'{x:02x}' for x in result)


def reset_mac(tgt, port):
    """Reset DUT interface MAC address to default."""
    node = "infix-interfaces:custom-phys-address"
    xpath = iface.get_xpath(port, node)
    tgt.delete_xpath(xpath)


with infamy.Test() as test:
    CMD = "jq -r '.[\"mac-address\"]' /run/system.json"

    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        _, tport = env.ltop.xlate("target", "data")
        pmac = iface.get_phys_address(target, tport)
        cmac = tgtssh.runsh(CMD).stdout.strip()
        STATIC = "02:01:00:c0:ff:ee"
        OFFSET = "00:00:00:00:ff:aa"

        print(f"Chassis MAC address target: {cmac}")
        print(f"Default MAC address of {tport} : {pmac}")

    with test.step("Set target:data static MAC address '02:01:00:c0:ff:ee'"):
        config = {
            "interfaces": {
                "interface": [{
                    "name": f"{tport}",
                    "custom-phys-address": {
                        "static": f"{STATIC}"
                    }
                }]
            }
        }
        target.put_config_dict("ietf-interfaces", config)

    with test.step("Verify target:data has MAC address '02:01:00:c0:ff:ee'"):
        mac = iface.get_phys_address(target, tport)
        print(f"Current MAC: {mac}, should be: {STATIC}")
        assert mac == STATIC

    with test.step("Reset target:data MAC address to default"):
        reset_mac(target, tport)

    with test.step("Verify target:data MAC address is reset to default"):
        until(lambda: iface.get_phys_address(target, tport) == pmac)
        
    with test.step("Set target:data to chassis MAC"):
        config = {
            "interfaces": {
                "interface": [{
                    "name": f"{tport}",
                    "custom-phys-address": {
                        "chassis": {}
                    }
                }]
            }
        }
        target.put_config_dict("ietf-interfaces", config)

    with test.step("Verify target:data has chassis MAC"):
        mac = iface.get_phys_address(target, tport)
        print(f"Current MAC: {mac}, should be: {cmac}")
        assert mac == cmac

    with test.step("Set target:data to chassis MAC + offset"):
        print(f"Setting chassis MAC {cmac} + offset {OFFSET}")
        config = {
            "interfaces": {
                "interface": [{
                    "name": f"{tport}",
                    "custom-phys-address": {
                        "chassis": {
                            "offset": f"{OFFSET}"
                        }
                    }
                }]
            }
        }
        target.put_config_dict("ietf-interfaces", config)

    with test.step("Verify target:data has chassis MAC + offset"):
        mac = iface.get_phys_address(target, tport)
        BMAC = calc_mac(cmac, OFFSET)
        print(f"Current MAC: {mac}, should be: {BMAC} (calculated)")
        assert mac == BMAC

    with test.step("Reset target:data MAC address to default"):
        reset_mac(target, tport)

    with test.step("Verify target:data MAC address is reset to default"):
        until(lambda: iface.get_phys_address(target, tport) == pmac)

        
    test.succeed()
