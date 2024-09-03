#!/usr/bin/env python3

# Test that it is possible to get all operational data

import infamy
import infamy.iface as iface

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Get all Operational data"):
        target.get_data(parse=False)

    test.succeed()
