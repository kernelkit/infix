#!/usr/bin/env python3
"""Schedule Reboot

Verify that it is possible to schedule a system reboot using the
infix-schedule module.
"""
import infamy
from infamy.util import wait_boot

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt", "netconf")

    with test.step("Schedule a reboot"):
        target.put_config_dicts({
            "ietf-system": {
                "system": {
                    "infix-schedule:schedules": {
                        "schedule": [
                            {
                                "name": "reboot-test",
                                "enabled": True,
                                "recurrence": {
                                    "frequency": "ietf-schedule:minutely",
                                    "interval": 1
                                }
                            }
                        ]
                    },
                    "infix-system:scheduled-reboot": {
                        "schedule": "reboot-test"
                    }
                }
            }
        })

    with test.step("Wait for reboot"):
        if not wait_boot(target, env):
            test.fail("System did not reboot as expected")

    with test.step("Verify system is back up"):
        target = env.attach("target", "mgmt", "netconf")

    test.succeed()
