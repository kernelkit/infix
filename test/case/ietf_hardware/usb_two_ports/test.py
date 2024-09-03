#!/usr/bin/env python3
"""
USB configuration with two USB ports

Test that the configuration is consistent
when having two USB ports.
"""
import infamy
import copy
import infamy.usb as usb
import time
import infamy.netconf as netconf
from infamy.util import until,wait_boot

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        available=usb.get_usb_ports(target)

        if len(available) < 2:
            test.skip()

    with test.step("Lock the first  USB port, and unlock the second USB port"):
        components = []
        component = {
            "name": available[0],
            "class": "infix-hardware:usb",
            "state": {
                "admin-state": "locked"
            }
        }
        components.append(component)
        component = {
            "name": available[1],
            "class": "infix-hardware:usb",
            "state": {
                "admin-state": "unlocked"
            }
        }
        components.append(component)

        target.put_config_dict("ietf-hardware", {
            "hardware": {
                "component": components
            }
        })
    with test.step("Verify that the correct port is locked and the correct one is unlocked"):
        until(lambda: usb.get_usb_state(target, available[0]) == "locked")
        until(lambda: usb.get_usb_state(target, available[1]) == "unlocked")

    with test.step("Unlock the first USB port, and lock the second USB port"):
        components = []
        component = {
            "name": available[0],
            "class": "infix-hardware:usb",
            "state": {
                "admin-state": "unlocked"
            }
        }
        components.append(component)
        component = {
            "name": available[1],
            "class": "infix-hardware:usb",
            "state": {
                "admin-state": "locked"
            }
        }
        components.append(component)

        target.put_config_dict("ietf-hardware", {
            "hardware": {
                "component": components
            }
        })
    with test.step("Verify that the correct port is locked and the correct one is unlocked"):
        until(lambda: usb.get_usb_state(target, available[0]) == "unlocked")
        until(lambda: usb.get_usb_state(target, available[1]) == "locked")
    test.succeed()
