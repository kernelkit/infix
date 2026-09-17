#!/usr/bin/env python3
"""Prune stray podman containers on all DUTs.

Workaround for the test rig: other tests may leave stray containers
behind on the DUTs, and Infix cannot prune them itself due to
limitations in podman.  Until that is fixed upstream we simply prune all
stopped containers before running the test suites. This is mainly a
problem on the test rig where other tests has left stray containers.

See Infix issue "Stray containers are not pruned":
https://github.com/kernelkit/infix/issues/1614
"""
import infamy

with infamy.Test() as test:
    with test.step("Discover topology and attach to available DUTs"):
        env = infamy.Env(False)
        ctrl = env.ptop.get_ctrl()
        duts = {}
        for ix in env.ptop.get_infixen():
            cport, ixport = env.ptop.get_mgmt_link(ctrl, ix)
            print(f"Attaching to {ix}:{ixport} via {ctrl}:{cport}")
            duts[ix] = env.attach(ix, ixport, protocol="ssh", test_reset=False)

    with test.step("Prune stopped containers"):
        for name, tgt in duts.items():
            print(f"{name}: pruning containers")
            rc = tgt.runsh("sudo rm -rf /var/lib/containers")
            print(rc.stdout)
            if rc.returncode != 0:
                test.fail()

    test.succeed()
