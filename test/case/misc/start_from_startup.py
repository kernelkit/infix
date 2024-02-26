#!/usr/bin/env python3

import infamy
from infamy.util import wait_boot
import copy

def remove_config(target):
    running = target.get_config_dict("/ietf-hardware:hardware")
    new = copy.deepcopy(running)
    new["hardware"].clear()
    target.put_diff_dicts("ietf-hardware",running,new)

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x1"))
        target = env.attach("target", "mgmt")

    with test.step("Configure"):
        target.put_config_dict("ietf-system", {
            "system": {
                "hostname": "test"
            }
        })
        remove_config(target)
        target.copy("running", "startup")
    with test.step("Reboot and wait for the unit to come back"):
        target.copy("running", "startup")
        target.reboot()
        if wait_boot(target) == False:
            test.fail()
        target = env.attach("target", "mgmt", factory_default = False)

    with test.step("Verify hostname"):
        data = target.get_dict("/ietf-system:system/hostname")
        print(data)
        assert(data["system"]["hostname"] == "test")

    test.succeed()
