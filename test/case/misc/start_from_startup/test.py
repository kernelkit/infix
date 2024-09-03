#!/usr/bin/env python3

import infamy
from infamy.util import wait_boot
import copy

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Configure"):
        target.put_config_dict("ietf-system", {
            "system": {
                "hostname": "test"
            }
        })
        target.delete_xpath("/ietf-hardware:hardware/component")
        target.copy("running", "startup")
    with test.step("Reboot and wait for the unit to come back"):
        target.startup_override()
        target.copy("running", "startup")
        target.reboot()
        if wait_boot(target) == False:
            test.fail()
        target = env.attach("target", "mgmt", test_default = False)

    with test.step("Verify hostname"):
        data = target.get_dict("/ietf-system:system/hostname")
        assert(data["system"]["hostname"] == "test")

    test.succeed()
