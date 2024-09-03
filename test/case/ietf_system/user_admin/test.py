#!/usr/bin/env python3
"""
  - Add a non-admin user and verify they have no privs
  - Add user to admin group and recheck privileges
  - Test admin user, verify $factory$ password in running
    and active password in operational datastore
"""

import infamy
import infamy.ssh as ssh
from passlib.hash import sha256_crypt

with infamy.Test() as test:
    with test.step("Initializing ..."):
        env = infamy.Env()
        target = env.attach("target", "mgmt")
        tgtssh = env.attach("target", "mgmt", "ssh")
        factory = env.get_password("target")
        address = target.get_mgmt_ip()

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
        # Don't presume 'admin' user exists, only group
        nacm = target.get_config_dict("/ietf-netconf-acm:nacm")
        for group in nacm["nacm"]["groups"]["group"]:
            if group["name"] == "admin":
                if USER not in group["user-name"]:
                    group["user-name"].append(USER)
        target.put_config_dict("ietf-netconf-acm", nacm)

    with test.step(f"Verify user {USER} is now in wheel group ..."):
        if not tgtssh.runsh(f"grep wheel /etc/group | grep '{USER}'"):
            test.fail()

    with test.step(f"Verify user {USER} shell now is Bash ..."):
        user = tgtssh.runsh(f"grep {USER} /etc/passwd").stdout
        if "bash" not in user:
            test.fail()

    with test.step(f"Change user {USER} to $factory$ password ..."):
        running = target.get_config_dict("/ietf-system:system")
        users = running["system"]["authentication"]["user"]

        for user in users:
            if user['name'] == USER:
                user['password'] = "$factory$"
                break
        target.put_config_dict("ietf-system", running)

    with test.step(f"Verify user {USER} exists and has new password ..."):
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

    with test.step(f"Verify user {USER} can log in with SSH ..."):
        conn = ssh.Device(ssh.Location(address, factory, USER))
        try:
            pwd = conn.runsh(f"cat /etc/passwd | grep {USER}").stdout
            print(f"Found {pwd.rstrip()}")
        except Exception as err:
            print(f"Failed connecting to target as {USER}: {err}")
            test.fail()

    test.succeed()
