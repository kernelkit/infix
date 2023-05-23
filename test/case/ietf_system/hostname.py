#!/usr/bin/env python3

import random, string

import infamy

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x1"))
        target = env.attach("target", "mgmt")

    with test.step("Set new hostname"):
        new = "".join((random.choices(string.ascii_lowercase, k=16)))

        target.put_config_dict("ietf-system", {
            "system": {
                "hostname": new,
            }
        })

    with test.step(f"Verify new hostname ({new})"):
        running = target.get_config_dict("/ietf-system:system")
        assert(running["system"]["hostname"] == new)

    test.succeed()
