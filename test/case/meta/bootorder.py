#!/usr/bin/env python3
import infamy
from infamy.util import parallel, until


def booted(tgt):
    data = tgt.get_data("/ietf-system:system-state") or {}
    return data.get("system-state", {}).get("software", {}).get("booted")


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

    with test.step("Verify bootorder"):
        for name, tgt in duts.items():
            expected = env.ptop.get_expected_boot(name)
            running  = until(lambda: booted(tgt), 30)
            print(f"{name}: booted: {running} expected: {expected}")

            if running != expected:
                test.fail()
    test.succeed()
