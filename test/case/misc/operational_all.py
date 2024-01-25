#!/usr/bin/env python3

# Test that it is possible to get all operational data

import infamy
import infamy.iface as iface

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x1"))
        target = env.attach("target", "mgmt")

    with test.step("Get all Operational data"):
        target.get_data(as_xml=True)

    test.succeed()
