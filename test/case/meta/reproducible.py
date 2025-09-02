#!/usr/bin/env python3

import os

import infamy

with infamy.Test() as test:
    with test.step("$PYTHONHASHSEED is set"):
        seed = os.environ.get("PYTHONHASHSEED")
        if seed == None:
            print("$PYTHONHASHSEED must be set in order to create a reproducible test environment")
            test.fail()
        else:
            print(f"Specify PYTHONHASHSEED={seed} to reproduce this test environment")

    test.succeed()
