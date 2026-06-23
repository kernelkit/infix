#!/usr/bin/env python3
"""Boot From Factory Config

Verify that the device boots cleanly from its factory-config and stays
usable, i.e. it does not fall back to the fail-secure failure-config.
Clearing the startup-config makes confd bootstrap running from the
factory-config on the next boot, as on a factory-fresh device.
"""

import json

import infamy
from infamy.util import wait_boot

STARTUP = "/cfg/startup-config.cfg"
FACTORY = "/etc/factory-config.cfg"


def factory_hostname(tgtssh):
    """Read the hostname the factory-config will boot with."""
    cfg = json.loads(tgtssh.runsh(f"cat {FACTORY}").stdout)
    return cfg.get("ietf-system:system", {}).get("hostname")


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")

    with test.step("Determine factory-config hostname"):
        expected = factory_hostname(tgtssh)
        assert expected, "Could not read hostname from factory-config"
        print(f"Factory config hostname is {expected!r}")

    with test.step("Clear startup-config so the device boots from factory"):
        # No startup-config on the startup boot path -> confd bootstraps
        # running from the factory-config.
        tgtssh.runsh(f"rm -f {STARTUP}")
        target.startup_override()

    with test.step("Reboot onto the factory config"):
        target.reboot()
        if not wait_boot(target, env):
            test.fail("Device did not boot from factory config")

    with test.step("Verify device is usable and not in failure-config"):
        target = env.attach("target", "mgmt", test_reset=False)

        # A failed bootstrap reverts to failure-config, which has a
        # different hostname; matching the factory hostname proves we
        # booted on the factory config, not the fail-secure fallback.
        running = target.get_config_dict("/ietf-system:system")
        assert running.get("system", {}).get("hostname") == expected, \
            "Device did not boot on the factory config (failure-config fallback?)"

    test.succeed()
