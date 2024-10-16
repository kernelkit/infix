#!/usr/bin/env python3

# Test that it is possible to get all operational data
"""
Get operational

Basic test just to get operational from test-config without errors.
"""
import infamy
import infamy.iface as iface

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Copy test-config to running configuration"):
        pass

    with test.step("Get all Operational data from 'target', verify there are no errors"):
        target.get_data(parse=False)

    test.succeed()
