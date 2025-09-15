#!/usr/bin/env python3
"""Set hostname

Verify that it is possible to set hostname using both regular text and
format specificers, like '%h-%m'.  These format specifiers expand to
'<default hostname>-<MAC>', where MAC is the last three bytes of the
base MAC address.  E.g., ix-01-01-01.

"""
import re
import infamy

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        FMT = "%h-%m"
        NEW = "h0stn4m3"

    with test.step("Set hostname to 'h0stn4m3'"):
        target.put_config_dict("ietf-system", {
            "system": {
                "hostname": NEW,
            }
        })

    with test.step("Verify new hostname 'h0stn4m3'"):
        running = target.get_config_dict("/ietf-system:system")
        assert running["system"]["hostname"] == NEW

    with test.step("Set hostname to '%h-%m'"):
        target.put_config_dict("ietf-system", {
            "system": {
                "hostname": FMT,
            }
        })

    with test.step("Verify hostname is  %h-%m in running configuration"):
        running = target.get_config_dict("/ietf-system:system")
        if running["system"]["hostname"] != FMT:
            test.fail()

    with test.step("Verify hostname format in operational"):
        cmd = tgtssh.runsh("sed -n s/^DEFAULT_HOSTNAME=//p /etc/os-release")
        default = cmd.stdout.rstrip()

        oper = target.get_data("/ietf-system:system")
        name = oper["system"]["hostname"]
        pattern = rf'^{default}-([0-9a-fA-F]{{2}}-){{2}}[0-9a-fA-F]{{2}}$'
        regex = re.compile(pattern)
        if not regex.match(name):
            test.fail()

    test.succeed()
