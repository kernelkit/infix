#!/usr/bin/env python3
import infamy
from infamy.util import parallel
import os

with infamy.Test() as test:
    with test.step("Discover topology and attach to available DUTs"):
        env = infamy.Env(False)
        ctrl = env.ptop.get_ctrl()

        def attach(ix):
            cport, ixport = env.ptop.get_mgmt_link(ctrl, ix)
            print(f"Attaching to {ix}:{ixport} via {ctrl}:{cport}")
            return env.attach(ix, ixport)

        infixen = env.ptop.get_infixen()
        duts = dict(zip(infixen, parallel(*(lambda ix=ix: attach(ix)
                                            for ix in infixen))))

    with test.step("Verify software version"):
        expected=os.environ.get("VERSION")
        for name, tgt in duts.items():
            running = tgt.get_data("/ietf-system:system-state")
            running = running['system-state']['platform']['os-version']
            print(f"{name}: booted: {running} expected {expected}")
            if running != expected:
                test.fail()
    test.succeed()
