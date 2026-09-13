#!/usr/bin/env python3
"""Support data collection

Verify that the support-collect RPC returns a valid archive with the
expected content, that the archive can be GPG encrypted, and that NACM
denies the RPC to users outside the admin group.

"""

import base64
import json
import os
import shutil
import subprocess
import tarfile
import tempfile
import infamy

PASSWORD = "test-support-password-123"
NOBODY = "supportless"
NOBODY_PASSWORD = "supportless-password"
# yescrypt hash of NOBODY_PASSWORD
NOBODY_HASH = "$y$j9T$SALT$bfLUDwjZLjCLQMpOHuw2hOuM6tJAXRcp3awucsABnN2"

EXPECTED = [
    "collection.log",
    "running-config.json",
    "operational-config.json",
    "system/dmesg.txt",
    "system/meminfo.txt",
    "network/ip/addr.json",
]


def verify(local, expected):
    with tarfile.open(local, "r:gz") as tar:
        members = tar.getnames()
        if not members:
            raise Exception("archive is empty")

        root = members[0].split("/")[0]
        print(f"Archive {root} contains {len(members)} files/directories")

        missing = [e for e in expected if f"{root}/{e}" not in members]
        if missing:
            raise Exception(f"missing from archive: {', '.join(missing)}")

        for name in ("running-config.json", "operational-config.json"):
            with tar.extractfile(f"{root}/{name}") as f:
                try:
                    json.load(f)
                except json.JSONDecodeError as e:
                    raise Exception(f"{name} in archive is not valid JSON,"
                                    f" collection of it failed: {e}")


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        local = {}
        for name in ("archive", "encrypted", "decrypted"):
            fd, path = tempfile.mkstemp(prefix=f"support-{name}-")
            os.close(fd)
            local[name] = path

        def cleanup():
            for path in local.values():
                if os.path.exists(path):
                    os.remove(path)

        test.push_test_cleanup(cleanup)

    with test.step("Collect support data with the support-collect RPC"):
        output = target.rpc_output("infix-system", "support-collect")

    with test.step("Verify the archive returned by the RPC"):
        if "data" not in output:
            raise Exception(f"RPC returned no inline archive: {output}")

        raw = base64.b64decode(output["data"])
        if len(raw) != int(output["size"]):
            raise Exception(f"RPC reported {output['size']} bytes,"
                            f" archive is {len(raw)}")

        print(f"RPC returned {len(raw)} bytes")
        with open(local["archive"], "wb") as f:
            f.write(raw)

        verify(local["archive"], EXPECTED)

    with test.step("Add user 'supportless', outside the admin NACM group"):
        target.put_config_dicts({
            "ietf-system": {
                "system": {
                    "authentication": {
                        "user": [
                            {
                                "name": NOBODY,
                                "password": NOBODY_HASH,
                                "shell": "infix-system:bash"
                            }
                        ]
                    }
                }
            }
        })
        test.push_test_cleanup(
            lambda: target.delete_xpath("/ietf-system:system/authentication"
                                        f"/user[name='{NOBODY}']"))

    with test.step("Verify user 'supportless' is denied the support-collect RPC"):
        other = env.attach("target", "mgmt", test_reset=False,
                           username=NOBODY, password=NOBODY_PASSWORD)
        try:
            other.rpc_output("infix-system", "support-collect")
        except Exception as e:
            print(f"Denied, as expected: {e}")
        else:
            raise Exception(f"{NOBODY} was allowed to collect support data")

    with test.step("Collect an encrypted archive with the support-collect RPC"):
        try:
            output = target.rpc_output("infix-system", "support-collect",
                                       {"password": PASSWORD})
        except Exception as e:
            if "gpg is not available" not in str(e):
                raise
            print("GPG not available on target - skipping encryption test")
            output = None

    with test.step("Decrypt the encrypted archive and verify it"):
        if output is None:
            print("Skipped, target has no gpg")
        elif not shutil.which("gpg"):
            print("Warning: gpg not available on host - skipping decrypt")
        else:
            with open(local["encrypted"], "wb") as f:
                f.write(base64.b64decode(output["data"]))

            with open(local["encrypted"], "rb") as ef, \
                 open(local["decrypted"], "wb") as df:
                result = subprocess.run(
                    ["gpg", "--batch", "--yes", "--passphrase", PASSWORD,
                     "--pinentry-mode", "loopback", "-d"],
                    stdin=ef, stdout=df, stderr=subprocess.PIPE, timeout=60)

            if result.returncode != 0:
                raise Exception("failed to decrypt support data:"
                                f" {result.stderr.decode(errors='replace')}")

            verify(local["decrypted"], EXPECTED)

    test.succeed()
