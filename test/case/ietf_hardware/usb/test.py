#!/usr/bin/env python3
"""USB configuration

This test checks if the configuration is consistent with hardware state,
and verifies whether the USB ports are correctly _locked_ (restricted from
use) and _unlocked_ (available for use) when they should. It also verifies
this behavior during reboot. This test does not involve the actual use of
the USB port; it only ensures the configured state is consistent with the
hardware state.

If this pass you can be certain that the configuration of the USB
port is handled correctly.

"""
import infamy
import copy
import infamy.usb as usb
import time
import infamy.netconf as netconf
from infamy.util import until, wait_boot

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        available = usb.get_usb_ports(target)

        if len(available) < 1:
            test.skip()

    with test.step("Unlock all USB ports"):
        components = []
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

    with test.step("Verify that all USB ports are unlocked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "unlocked")

    with test.step("Lock all USB ports"):
        components = []
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

    with test.step("Verify that all USB ports are locked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "locked")

    with test.step("Remove all hardware configuration"):
        for port in available:
            target.delete_xpath(f"/ietf-hardware:hardware/component[name='{port}']")

    with test.step("Verify that all USB ports are locked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "locked")

    with test.step("Unlock USB ports"):
        components = []
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

    with test.step("Verify that all USB ports are unlocked"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "unlocked")

    with test.step("Save the configuration to startup configuration and reboot"):
        target.startup_override()
        target.copy("running", "startup")
        target.reboot()
        if not wait_boot(target, env):
            test.fail()
        target = env.attach("target", "mgmt", test_reset=False)

    with test.step("Verify USB port remain unlocked after reboot"):
        for port in available:
            until(lambda: usb.get_usb_state(target, port) == "unlocked")

    test.succeed()
