#!/usr/bin/env python3

import infamy
from infamy.util import wait_boot

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Configure"):
        target.copy("running", "startup")

    with test.step("Reboot and wait for the unit to come back"):
        target.startup_override()
        target.copy("running", "startup")
        target.reboot()
        if not wait_boot(target, env):
            test.fail()
        target = env.attach("target", "mgmt", test_reset=False)
        tgtssh = env.attach("target", "mgmt", "ssh")

    with test.step("Verify user admin is now in wheel group"):
        if not tgtssh.runsh("grep wheel /etc/group | grep 'admin'"):
            test.fail()

    with test.step("Verify user admin is now in sys-cli group"):
        if not tgtssh.runsh("grep sys-cli /etc/group | grep 'admin'"):
            test.fail()

    test.succeed()
