#!/usr/bin/env python3
"""Boot From Factory Config

Verify that the device boots cleanly from its factory-config and stays
usable, i.e. it does not fall back to the fail-secure failure-config.
Clearing the startup-config makes confd bootstrap running from the
factory-config on the next boot, as on a factory-fresh device.

A factory-config may bridge the management port with the links to the
other devices, so every non-management port of every device in the
physical topology is disabled first, leaving the device alone on the
host's management port.
"""

import infamy
from infamy.util import parallel, until, wait_boot

STARTUP = "/cfg/startup-config.cfg"


def factory_hostname(target):
    """Read the hostname the factory-config will boot with."""
    factory = target.get_factory("/ietf-system:system/hostname")
    return factory.print_dict().get("system", {}).get("hostname")

def isolate(env):
    """Disable every non-management port of every DUT in the physical topology."""
    ptop = env.ptop
    ctrl = ptop.get_ctrl()

    def down(node):
        _, mgmt = ptop.get_mgmt_link(ctrl, node)
        ports = {e[node] for _, _, e in ptop.g.edges(node, data=True)} - {mgmt}
        dev = env.attach(node, test_reset=False)
        ifaces = dev.get_config_dict("/ietf-interfaces:interfaces")
        ports &= {i["name"] for i in ifaces["interfaces"]["interface"]}
        dev.put_config_dicts({"ietf-interfaces": {"interfaces": {
            "interface": [{"name": p, "enabled": False} for p in sorted(ports)]
        }}})

    nodes = sorted(ptop.get_infixen())
    parallel(*(lambda n=n: down(n) for n in nodes))

def cleanup(env):
    """Restore the rig to the clean per-test baseline for the next test."""
    print("Restoring device to clean test baseline")
    target = env.attach("target", "mgmt", "netconf")
    target.reboot()
    if not wait_boot(target, env):
        test.fail("Device did not come back while restoring baseline")

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target, tgtssh = parallel(lambda: env.attach("target", "mgmt"),
                                  lambda: env.attach("target", "mgmt", "ssh"))

    with test.step("Determine factory-config hostname"):
        expected = factory_hostname(target)
        assert expected, "Could not read hostname from factory-config"
        print(f"Factory config hostname is {expected!r}")

    with test.step("Disable all non-management ports on every DUT in the physical topology"):
        isolate(env)

    with test.step("Clear startup-config so the device boots from factory"):
        # No startup-config on the startup boot path -> confd bootstraps
        # running from the factory-config.
        until(lambda: tgtssh.runsh(f"rm -f {STARTUP}").returncode == 0)
        target.startup_override()

    with test.step("Reboot onto the factory config"):
        target.reboot()
        if not wait_boot(target, env):
            test.fail("Device did not boot from factory config")
        test.push_test_cleanup(lambda: cleanup(env))

    with test.step("Verify device is usable and not in failure-config"):
        target = env.attach("target", "mgmt", test_reset=False)

        # A failed bootstrap reverts to failure-config, which has a
        # different hostname; matching the factory hostname proves we
        # booted on the factory config, not the fail-secure fallback.
        running = target.get_config_dict("/ietf-system:system")
        assert running.get("system", {}).get("hostname") == expected, \
            "Device did not boot on the factory config (failure-config fallback?)"

    test.succeed()
