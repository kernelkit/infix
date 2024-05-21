#!/usr/bin/env python3
#

import random
import string

import infamy

def attach(env):
    return (env.attach("target", "mgmt"), env.attach("target", "mgmt", "ssh"))

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x1"))
        target, tgtssh = attach(env)

        cookie = "".join((random.choices(string.ascii_lowercase, k=16)))
        script = f"/cfg/user-scripts.d/infamy-user-script-test.sh"
        output = f"/tmp/{cookie}.cookie"

    with test.step(f"Install {script}"):
        tgtssh.runsh(f"""
        set -e

        mkdir -p $(dirname {script})
        printf '#!/bin/sh\necho -n {cookie} >{output}\n' >{script}
        chmod +x {script}
        """)

    with test.step("Enable user scripts and reboot"):
        target.put_config_dict("infix-services", {
            "user-scripts": {
                "enabled": True
            }
        })
        target.copy("running", "startup")
        target.reboot()
        infamy.util.wait_boot(target) or test.fail()
        target, tgtssh = attach(env)

    with test.step(f"Verify that {script} has run"):
        cat = tgtssh.runsh(f"cat {output}").stdout
        assert cat == cookie, f"Read back {repr(cat)}, expected {repr(cookie)}"

    with test.step("Disable user scripts and reboot"):
        target.put_config_dict("infix-services", {
            "user-scripts": {
                "enabled": False
            }
        })
        target.copy("running", "startup")
        target.reboot()
        infamy.util.wait_boot(target) or test.fail()
        target, tgtssh = attach(env)

    with test.step(f"Verify that {script} has not run"):
        exists = tgtssh.run(["test", "-f", f"{output}"]).returncode == 0
        assert not exists, f"Unexpectedly found {output}"

    with test.step(f"Remove {script}"):
        tgtssh.runsh(f"rm {script}")

    test.succeed()
