#!/usr/bin/env python3
"""Watchdog reset on system lockup

Verify that a system's watchdog trips and successfully reboots the
system back to a working state if a lockup occurs.

This is tested by using the Linux kernel's `test_lockup` module to
inject a hard lockup (i.e., blocking servicing of all interrupts) on
all CPU cores that lasts for twice as long as the watchdog's reported
timeout.

"""
import base64
import infamy
import json
import subprocess
import time

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")

    with test.step("Verify the presence of a watchdog device"):
        wctl = tgtssh.run(["watchdogctl", "-j"], stdout=subprocess.PIPE)
        conf = json.loads(wctl.stdout)

        dogs = [ dog for dog in conf.get("device", []) if dog.get("name", "") == "/dev/watchdog" ]
        if len(dogs) < 1:
            test.fail("No watchdog device available")
        else:
            dog = dogs[0]

    print(f"Found {dog['name']} ({dog['identity']}), timeout:{dog['timeout']}s")

    with test.step("Verify the presence of the test_lockup module"):
        if tgtssh.run(["modprobe", "-q", "-n", "test_lockup"]).returncode != 0:
            test.fail("test_lockup module is not available")

    with test.step("Trigger a hard lockup on all CPU cores"):
        tgtssh.runsh(f"""
        lockup()
        {{
            # Give the SSH session some time to properly shut down
            sleep 3

            sudo modprobe test_lockup \
                disable_irq=1 \
                all_cpus=1 \
                time_secs={dog['timeout'] * 2}
        }}

        lockup </dev/null &>/dev/null &
        """)

    with test.step("Wait for the watchdog to trip"):
        time.sleep(dog["timeout"])

    with test.step("Verify that the system reboots"):
        infamy.util.wait_boot(target, env)

    test.succeed()
