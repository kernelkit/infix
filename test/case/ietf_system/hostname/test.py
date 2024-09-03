#!/usr/bin/env python3
"""
Set hostname

Verify that it is possible to change hostname
"""
import random
import string
import re
import infamy

with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        fmt = "%h-%m"

    with test.step("Set new hostname"):
        new = "".join((random.choices(string.ascii_lowercase, k=16)))

        target.put_config_dict("ietf-system", {
            "system": {
                "hostname": new,
            }
        })

    with test.step(f"Verify new hostname ({new})"):
        running = target.get_config_dict("/ietf-system:system")
        assert running["system"]["hostname"] == new

    with test.step(f"Set hostname format: {fmt}"):
        target.put_config_dict("ietf-system", {
            "system": {
                "hostname": fmt,
            }
        })

    with test.step(f"Verify hostname format in running: {fmt}"):
        running = target.get_config_dict("/ietf-system:system")
        if running["system"]["hostname"] != fmt:
            test.fail()

        cmd = tgtssh.runsh("sed -n s/^DEFAULT_HOSTNAME=//p /etc/os-release")
        default = cmd.stdout.rstrip()

    with test.step(f"Verify hostname format in operational: {default}-c0-ff-ee"):

        oper = target.get_data("/ietf-system:system")
        name = oper["system"]["hostname"]
        pattern = rf'^{default}-([0-9a-fA-F]{{2}}-){{2}}[0-9a-fA-F]{{2}}$'
        regex = re.compile(pattern)
        if not regex.match(name):
            test.fail()

    test.succeed()
