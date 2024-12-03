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

    with test.step("Discover topology and attach to available DUTs"):
        env = infamy.Env(False)
        ctrl = env.ptop.get_ctrl()

        duts = {}
        for ix in env.ptop.get_infixen():
            cport, ixport = env.ptop.get_mgmt_link(ctrl, ix)
            print(f"Attaching to {ix}:{ixport} via {ctrl}:{cport}")
            duts[ix] = env.attach(ix, ixport)

    with test.step("Log running software versions"):
        for name, tgt in duts.items():
            sys = tgt.get_data("/ietf-system:system-state")
            sw = sys["system-state"]["software"]
            plt = sys["system-state"]["platform"]

            print(f"{name}:")
            for k,v in plt.items():
                print(f"  {k:<16s}  {v}")

            for k in ("compatible", "booted"):
                print(f"  {k:<16s}  {sw[k]}")

    test.succeed()
