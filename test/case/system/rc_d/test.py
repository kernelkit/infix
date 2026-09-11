#!/usr/bin/env python3
"""Boot scripts (rc.d)

Verify that user scripts stored in the configuration, under
/system/advanced/rc.ds, are extracted to /etc/rc.d and run once at boot,
in the configured order, and only when enabled.

Two enabled scripts each append their name to a file in /run, a third
script is disabled and must not run.  After saving to startup-config
and rebooting, the file must list the two names in configured order.
"""
import base64

import infamy
from infamy.util import parallel, wait_boot

ORDER = "/run/rc.d-test-order"
NEVER = "/run/rc.d-test-disabled"


def script(text):
    return base64.b64encode(text.encode()).decode()


def cleanup(env):
    print("Removing rc.d test scripts from startup-config")
    target = env.attach("target", "mgmt", test_reset=False)
    target.delete_xpath("/ietf-system:system/infix-system:advanced")
    target.copy("running", "startup")


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Configure two enabled scripts and one disabled"):
        target.put_config_dicts({
            "ietf-system": {
                "system": {
                    "infix-system:advanced": {
                        "rc.ds": {
                            "rc.d": [
                                {
                                    "name": "first",
                                    "content": script(f"#!/bin/sh\necho first >> {ORDER}\n"),
                                },
                                {
                                    "name": "second",
                                    "content": script(f"echo second >> {ORDER}\n"),
                                },
                                {
                                    "name": "never",
                                    "enabled": False,
                                    "content": script(f"touch {NEVER}\n"),
                                },
                            ],
                        }
                    }
                }
            }
        })

    with test.step("Verify scripts are extracted to /etc/rc.d in order"):
        tgtssh = env.attach("target", "mgmt", "ssh")
        files = tgtssh.runsh("ls /etc/rc.d").stdout.split()
        assert files == ["01-first", "02-second"], f"unexpected /etc/rc.d contents: {files}"

    with test.step("Save to startup-config and reboot"):
        target.startup_override()
        target.copy("running", "startup")
        test.push_test_cleanup(lambda: cleanup(env))
        target.reboot()
        if not wait_boot(target, env):
            test.fail()
        target, tgtssh = parallel(lambda: env.attach("target", "mgmt", test_reset=False),
                                  lambda: env.attach("target", "mgmt", "ssh"))

    with test.step("Verify enabled scripts ran once, in order"):
        out = tgtssh.runsh(f"cat {ORDER}").stdout.split()
        assert out == ["first", "second"], f"unexpected run order: {out}"

    with test.step("Verify disabled script did not run"):
        assert tgtssh.runsh(f"test -e {NEVER}").returncode != 0, "disabled script was run"

    test.succeed()
