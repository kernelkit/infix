#!/usr/bin/env python3

import infamy
import copy
from passlib.hash import sha256_crypt
import random
import string
import re


def generate_restrictred_credential():
    credential = "".join(random.choices(string.ascii_lowercase, k=64))

    while not re.match("^(?![0-9])[\w]+$", credential):
        credential = "".join(random.choices(string.ascii_lowercase, k=64))

    return credential


with infamy.Test() as test:
    with test.step("Initialize"):
        env = infamy.Env(infamy.std_topology("1x1"))
        target = env.attach("target", "mgmt")

    with test.step("Add new user"):
        username = generate_restrictred_credential()
        password = generate_restrictred_credential()
        hashed_password = sha256_crypt.using(rounds=5000).hash(password)
        print(f"username: {username}")
        print(f"password: {password}")
        print(f"hashed_password: {hashed_password}")

        target.put_config_dict("ietf-system", {
            "system": {
                "authentication": {
                    "user": [
                        {
                            "name": username,
                            "password": hashed_password,
                        }
                    ]
                }
            }
        })

    with test.step(f"Verify user ({username} / {hashed_password})"):
        running = target.get_config_dict("/ietf-system:system")
        users = running["system"]["authentication"]["user"]
        user_found = False
        for user in users:
            if user["name"] == username:
                user_found = True
                assert user["password"] == hashed_password
                break
        assert user_found, f"User {username} not found"

    with test.step(f"Delete user ({username} / {hashed_password})"):
        target.delete_xpath(f"/ietf-system:system/authentication/user[name='{username}']")

    with test.step(f"Verify erasure of user ({username} / {hashed_password})"):
        running = target.get_config_dict("/ietf-system:system")
        users = running["system"]["authentication"]["user"]
        for user in users:
            assert user["name"] != username, f"User {username} not deleted"

    test.succeed()
