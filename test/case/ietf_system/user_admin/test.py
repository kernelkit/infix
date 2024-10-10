#!/usr/bin/env python3
"""
Add admin user

Test that a non-admin user is not an admin in Linux, and
check that it when added as admin it is also the case in Linux.
"""

import infamy
import infamy.ssh as ssh
import infamy.util as util
from passlib.hash import sha256_crypt

with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        factory = env.get_password("target")
        address = target.get_mgmt_ip()

    with test.step("Add new user 'jacky' with no NACM access"):
        USER = "jacky"
        PASS = "$1$3aR7Bq2u$G9kV.8AALtKkCnaAXFyu6/"

        target.put_config_dict("ietf-system", {
            "system": {
                "authentication": {
                    "user": [
                        {
                            "name": USER,
                            "password": PASS,
                            "shell": "infix-system:bash"
                        }
                    ]
                }
            }
        })
        running = target.get_config_dict("/ietf-system:system")
        users = running["system"]["authentication"]["user"]

    with test.step("Verify regular user jacky exists"):
        jacky = next((user for user in users if user['name'] == USER), None)
        if not any(user['name'] == USER for user in users):
            test.fail()

    with test.step("Verify user jacky is not in wheel group"):
        wheel = tgtssh.runsh("grep wheel /etc/group").stdout
        if USER in wheel:
            test.fail()

    with test.step("Verify user jacky password is set correctly"):
        if not tgtssh.runsh(f"sudo grep ':{PASS}:' /etc/shadow"):
            test.fail()

    with test.step("Add user jacky to admin group in NACM"):
        # Don't presume 'admin' user exists, only group
        nacm = target.get_config_dict("/ietf-netconf-acm:nacm")
        for group in nacm["nacm"]["groups"]["group"]:
            if group["name"] == "admin":
                if USER not in group["user-name"]:
                    group["user-name"].append(USER)
        target.put_config_dict("ietf-netconf-acm", nacm)

    with test.step("Verify user jacky is now in wheel group"):
        if not tgtssh.runsh(f"grep wheel /etc/group | grep '{USER}'"):
            test.fail()

    with test.step("Verify user jacky shell now is Bash"):
        user = tgtssh.runsh(f"grep {USER} /etc/passwd").stdout
        if "bash" not in user:
            test.fail()

    with test.step("Change user jacky to $factory$ password ..."):
        running = target.get_config_dict("/ietf-system:system")
        users = running["system"]["authentication"]["user"]

        for user in users:
            if user['name'] == USER:
                user['password'] = "$factory$"
                break
        target.put_config_dict("ietf-system", running)

    with test.step("Verify user jacky exists and has new password"):
        operational = target.get_data("/ietf-system:system/authentication")
        users = operational["system"]["authentication"]["user"]

        found = None
        for user in users:
            if user['name'] == USER:
                found = user
                break

        if found is None:
            test.fail()
        if found['password'] == "$factory$":
            test.fail()
        if found['password'] == PASS:
            test.fail()

    with test.step("Verify user jacky can log in with SSH"):
        testssh=ssh.Device("target", ssh.Location(address, USER, factory))
        util.until(lambda: testssh.runsh("ls").returncode == 0)
    test.succeed()
