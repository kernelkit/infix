#!/usr/bin/env python3
import infamy

with infamy.Test() as test:
    with test.step("Discover topology and attach to available DUTs"):
        env = infamy.Env(False)
        ctrl = env.ptop.get_ctrl()
        duts = {}
        duts_state = {}
        for ix in env.ptop.get_infixen():
            cport, ixport = env.ptop.get_mgmt_link(ctrl, ix)
            print(f"Attaching to {ix}:{ixport} via {ctrl}:{cport}")
            duts[ix] = env.attach(ix, ixport)

    with test.step("Verify bootorder"):
        for name, tgt in duts.items():
            expected = env.ptop.get_expected_boot(name)
            running  = tgt.get_data("/ietf-system:system-state")
            running = running['system-state']['software']['booted']
            print(f"{name}: booted: {running} expected: {expected}")

            if running != expected:
                test.fail()
    test.succeed()
