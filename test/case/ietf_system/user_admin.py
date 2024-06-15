#!/usr/bin/env python3
"""
  Add a non-admin user and verify they have no privs
  Add user to admin group and recheck privileges
"""

import infamy
from passlib.hash import sha256_crypt

with infamy.Test() as test:
    with test.step("Initializing ..."):
        env = infamy.Env(infamy.std_topology("1x1"))
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")

    with test.step("Add new user"):
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

    with test.step(f"Verify regular user {USER} exists ..."):
        jacky = next((user for user in users if user['name'] == USER), None)
        if not any(user['name'] == USER for user in users):
            test.fail()

    with test.step(f"Verify user {USER} is not in wheel group ..."):
        wheel = tgtssh.runsh("grep wheel /etc/group").stdout
        if USER in wheel:
            test.fail()

    with test.step(f"Verify user {USER} shell is not Bash ..."):
        user = tgtssh.runsh(f"grep {USER} /etc/passwd").stdout
        if "bash" in user:
            test.fail()

    with test.step(f"Verify user {USER} password is set correctly ..."):
        if not tgtssh.runsh(f"sudo grep ':{PASS}:' /etc/shadow"):
            test.fail()

    with test.step(f"Add {USER} user to admin group"):
        target.put_config_dict("ietf-netconf-acm", {
            "nacm": {
                "groups": {
                    "group": [
                        {
                            "name": "admin",
                            "user-name": [
                                "admin",
                                "jacky"
                            ]
                        }
                    ]
                }
            }})

    with test.step(f"Verify user {USER} is now in wheel group ..."):
        if not tgtssh.runsh(f"grep wheel /etc/group | grep '{USER}'"):
            test.fail()

    with test.step(f"Verify user {USER} shell now is Bash ..."):
        user = tgtssh.runsh(f"grep {USER} /etc/passwd").stdout
        if "bash" not in user:
            test.fail()

    test.succeed()
