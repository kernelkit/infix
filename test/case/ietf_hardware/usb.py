#!/usr/bin/env python3

import infamy
import copy
import infamy.usb as usb
import time
import infamy.netconf as netconf
from infamy.util import until,wait_boot

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x1"))
        target = env.attach("target", "mgmt")
        available=usb.get_usb_ports(target)

        if len(available) < 1:
            test.skip()

    with test.step("Unlock USB ports"):
        components=[]
        for port in available:
            component = {
                "name": port,
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

    with test.step("Verify USB ports unlocked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "unlocked")

    if len(available) > 1:
        with test.step("Lock one port"):
            components=[]
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
        with test.step("Verify one port is locked and one unlocked"):
            until(lambda: usb.get_usb_state(target, available[1]) == "locked")
            until(lambda: usb.get_usb_state(target, available[0]) == "unlocked")

    with test.step("Lock USB ports"):
        components=[]
        for port in available:
            component = {
                "class": "infix-hardware:usb",
                "name": port,
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

    with test.step("Verify USB ports locked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "locked")
    with test.step("Remove all hardware configuration"):
        for port in available:
            target.delete_xpath(f"/ietf-hardware:hardware/component[name='{port}']")

    with test.step("Verify USB ports locked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "locked")


    with test.step("Unlock USB ports"):
        components=[]
        for port in available:
            component = {
                "name": port,
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

    with test.step("Verify USB ports unlocked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "unlocked")

    with test.step("Save to startup and reboot"):
        target.startup_override()
        target.copy("running", "startup")
        target.reboot()
        if wait_boot(target) == False:
            test.fail()
        target = env.attach("target", "mgmt", test_reset=False)

    with test.step("Verify that all ports are unlocked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "unlocked")
    test.succeed()
