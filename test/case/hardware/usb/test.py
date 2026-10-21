#!/usr/bin/env python3
"""USB configuration

This test checks if the configuration is consistent with hardware state,
and verifies whether the USB ports are correctly _locked_ (restricted from
use) and _unlocked_ (available for use) when they should. It also verifies
this behavior during reboot. This test does not involve the actual use of
the USB port; it only ensures the configured state is consistent with the
hardware state.  Removing the configuration of a port locks it.

The target has a USB stick plugged in.  It is expected to be listed
under its port the whole time: not authorized while the port is
locked, authorized as mass storage once the port is unlocked.

If this pass you can be certain that the configuration of the USB
port is handled correctly.

"""
import infamy
import infamy.usb as usb
from infamy.util import until, wait_boot


def set_usb_state(target, ports, state):
    target.put_config_dicts({"ietf-hardware": {"hardware": {"component": [
        {"name": port, "class": "infix-hardware:usb",
         "state": {"admin-state": state}} for port in ports
    ]}}})


def verify_usb_state(target, ports, state):
    for port in ports:
        until(lambda: usb.get_usb_state(target, port) == state)


def verify_usb_devices(target, devices, authorized):
    for name in devices:
        until(lambda: usb.get_usb_device(target, name).get("authorized") is authorized)


def verify_usb_stick(target, devices):
    until(lambda: any("mass-storage" in usb.get_usb_device(target, name).get("classes", [])
                      for name in devices))


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        available = usb.get_usb_ports(target)
        devices = [d["name"] for d in usb.get_usb_devices(target)
                   if d["parent"] in available]

        if len(available) < 1:
            test.skip()
        if not devices:
            test.fail()

    with test.step("Unlock all USB ports"):
        set_usb_state(target, available, "unlocked")

    with test.step("Verify that all USB ports are unlocked"):
        verify_usb_state(target, available, "unlocked")

    with test.step("Verify that the USB stick is authorized"):
        verify_usb_devices(target, devices, True)
        verify_usb_stick(target, devices)

    with test.step("Lock all USB ports"):
        set_usb_state(target, available, "locked")

    with test.step("Verify that all USB ports are locked"):
        verify_usb_state(target, available, "locked")

    with test.step("Verify that the USB stick is still listed, not authorized"):
        verify_usb_devices(target, devices, False)

    with test.step("Unlock all USB ports again"):
        set_usb_state(target, available, "unlocked")

    with test.step("Verify that all USB ports are unlocked"):
        verify_usb_state(target, available, "unlocked")

    with test.step("Remove all hardware configuration"):
        for port in available:
            target.delete_xpath(f"/ietf-hardware:hardware/component[name='{port}']")

    with test.step("Verify that all USB ports are locked"):
        verify_usb_state(target, available, "locked")

    with test.step("Unlock USB ports"):
        set_usb_state(target, available, "unlocked")

    with test.step("Verify that all USB ports are unlocked"):
        verify_usb_state(target, available, "unlocked")

    with test.step("Save the configuration to startup configuration and reboot"):
        target.startup_override()
        target.copy("running", "startup")
        target.reboot()
        if not wait_boot(target, env):
            test.fail()
        target = env.attach("target", "mgmt", test_reset=False)

    with test.step("Verify USB port remain unlocked after reboot"):
        verify_usb_state(target, available, "unlocked")

    test.succeed()
