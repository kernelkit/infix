#!/usr/bin/env python3
"""Support data collection

Verify that the support-collect RPC returns a valid archive with the
expected content, that private keys and login hashes are removed from
the configuration in it, that the archive can be GPG encrypted, and
that an archive too large to return inline is left on the device and
its path returned instead.

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
BIG_FILE = "/var/log/support-test.bin"
BIG_MB = 17
WORK_DIR = "/var/lib/support"

EXPECTED = [
    "collection.log",
    "running-config.json",
    "operational-config.json",
    "system/dmesg.txt",
    "system/meminfo.txt",
    "network/ip/addr.json",
]
SECRETS = ("password", "cleartext-private-key", "cleartext-symmetric-key",
           "shared-secret")


def secrets(node, found=None):
    """Collect (leaf, value) for every secret leaf in a config tree"""
    if found is None:
        found = []
    if isinstance(node, dict):
        for key, val in node.items():
            if key.split(":")[-1] in SECRETS and isinstance(val, str):
                found.append((key, val))
            else:
                secrets(val, found)
    elif isinstance(node, list):
        for val in node:
            secrets(val, found)
    return found


def free_kb(ssh, path):
    """Free space on the filesystem holding path, in KiB"""
    result = ssh.runsh(f"df -Pk {path} | awk 'NR == 2 {{ print $4 }}'")
    return int(result.stdout.strip())


def save(local, output):
    """Decode the archive in an RPC reply to a local file, return its size"""
    if "data" not in output:
        raise Exception(f"RPC returned no inline archive: {output}")

    raw = base64.b64decode(output["data"])
    if len(raw) != int(output["size"]):
        raise Exception(f"RPC reported {output['size']} bytes,"
                        f" archive is {len(raw)}")

    with open(local, "wb") as f:
        f.write(raw)

    return len(raw)


def verify_contents(local, expected):
    with tarfile.open(local, "r:gz") as tar:
        members = tar.getnames()
        if not members:
            raise Exception("archive is empty")

        root = members[0].split("/")[0]
        print(f"Archive {root} contains {len(members)} files/directories")

        missing = [e for e in expected if f"{root}/{e}" not in members]
        if missing:
            raise Exception(f"missing from archive: {', '.join(missing)}")


def config(local, name):
    """Load a JSON configuration file from the archive"""
    with tarfile.open(local, "r:gz") as tar:
        root = tar.getnames()[0].split("/")[0]
        with tar.extractfile(f"{root}/{name}") as f:
            try:
                return json.load(f)
            except json.JSONDecodeError as e:
                raise Exception(f"{name} in archive is not valid JSON,"
                                f" collection of it failed: {e}")


def admin_user(running):
    users = running.get("ietf-system:system", {}) \
                   .get("authentication", {}).get("user", [])
    admin = [u for u in users if u.get("name") == "admin"]
    if not admin:
        raise Exception("running-config.json has no admin user, "
                        f"users: {[u.get('name') for u in users]}")
    return admin[0]


def verify_keystore_and_admin(local):
    running = config(local, "running-config.json")
    if "ietf-keystore:keystore" not in running:
        raise Exception("running-config.json has no keystore, the factory "
                        "configuration has two keys in it")
    admin_user(running)
    print("running-config.json: keystore and admin user present")


def verify_login_hash_removed(local):
    admin = admin_user(config(local, "running-config.json"))
    if "password" in admin:
        raise Exception("running-config.json leaks the admin login hash: "
                        f"{admin['password']}")
    print("running-config.json: admin user has no password leaf")


def verify_no_secrets(local):
    for name in ("running-config.json", "operational-config.json"):
        leaked = [key for key, _ in secrets(config(local, name))]
        if leaked:
            raise Exception(f"{name} leaks secrets: {', '.join(leaked)}")
        print(f"{name}: no secret leaves")


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

        local = {}
        for name in ("archive", "encrypted", "decrypted", "big"):
            fd, path = tempfile.mkstemp(prefix=f"support-{name}-")
            os.close(fd)
            local[name] = path

        def cleanup():
            for path in local.values():
                if os.path.exists(path):
                    os.remove(path)

        test.push_test_cleanup(cleanup)

    with test.step("Call the infix-system:support-collect RPC without a password"):
        output = target.rpc_output("infix-system", "support-collect")

    with test.step("Base64 decode the 'data' reply to a .tar.gz file, verify "
                   "its length matches the 'size' reply"):
        size = save(local["archive"], output)
        print(f"RPC returned {size} bytes")

    with test.step("Verify the archive holds collection.log, running-config.json, "
                   "operational-config.json, system/dmesg.txt, system/meminfo.txt "
                   "and network/ip/addr.json"):
        verify_contents(local["archive"], EXPECTED)

    with test.step("Verify running-config.json in the archive has the "
                   "ietf-keystore:keystore container and the admin user"):
        verify_keystore_and_admin(local["archive"])

    with test.step("Verify the admin user in running-config.json has no "
                   "password leaf"):
        verify_login_hash_removed(local["archive"])

    with test.step("Verify neither running-config.json nor operational-config.json "
                   "has any password, cleartext-private-key, "
                   "cleartext-symmetric-key or shared-secret leaf"):
        verify_no_secrets(local["archive"])

    with test.step("Call the support-collect RPC with password "
                   "'test-support-password-123'"):
        try:
            output = target.rpc_output("infix-system", "support-collect",
                                       {"password": PASSWORD})
        except Exception as e:
            if "gpg is not available" not in str(e):
                raise
            print("GPG not available on target - skipping encryption test")
            output = None

    with test.step("Base64 decode the reply to a .gpg file, decrypt it with "
                   "gpg and the same password"):
        if output is None:
            print("Skipped, target has no gpg")
        elif not shutil.which("gpg"):
            raise Exception("gpg is required on the test host")
        else:
            save(local["encrypted"], output)

            with open(local["encrypted"], "rb") as ef, \
                 open(local["decrypted"], "wb") as df:
                result = subprocess.run(
                    ["gpg", "--batch", "--yes", "--passphrase", PASSWORD,
                     "--pinentry-mode", "loopback", "-d"],
                    stdin=ef, stdout=df, stderr=subprocess.PIPE, timeout=60)

            if result.returncode != 0:
                raise Exception("failed to decrypt support data:"
                                f" {result.stderr.decode(errors='replace')}")

    with test.step("Verify the decrypted archive holds the same files as the "
                   "first one"):
        if output is None:
            print("Skipped, target has no gpg")
        else:
            verify_contents(local["decrypted"], EXPECTED)

    with test.step("Verify the decrypted archive has the same secrets removed"):
        if output is None:
            print("Skipped, target has no gpg")
        else:
            verify_login_hash_removed(local["decrypted"])
            verify_no_secrets(local["decrypted"])

    with test.step("Attach to target over ssh and create /var/log/support-test.bin "
                   "with 17 MB of random data"):
        tgtssh = env.attach("target", "mgmt", "ssh", test_reset=False)

        free = free_kb(tgtssh, WORK_DIR)
        need = 2 * (size // 1024 + BIG_MB * 1024) + 2048
        room = free >= need
        print(f"{WORK_DIR}: {free // 1024} MB free, collecting {BIG_MB} MB of extra"
              f" logs needs about {need // 1024} MB")

        if not room:
            print("Skipped, no room on the device")
        else:
            test.push_test_cleanup(
                lambda: tgtssh.run(f"sudo rm -f {BIG_FILE}", check=False))
            tgtssh.run(f"sudo dd if=/dev/urandom of={BIG_FILE} bs=1M count={BIG_MB}",
                       check=True, capture_output=True)

    with test.step("Call the support-collect RPC, verify the reply has 'size' over "
                   "16 MiB and 'filename', but no inline 'data'"):
        if not room:
            print("Skipped, no room on the device")
        else:
            output = target.rpc_output("infix-system", "support-collect")
            if "data" in output or "filename" not in output:
                raise Exception("expected the archive left on the device,"
                                f" got {list(output)}")
            if int(output["size"]) <= 16 * 1024 * 1024:
                raise Exception(f"archive is {output['size']} bytes, not over 16 MiB")

            remote = output["filename"]
            test.push_test_cleanup(
                lambda: tgtssh.run(f"sudo rm -f {remote}", check=False))
            print(f"Archive of {output['size']} bytes left at {remote}")

    with test.step("Fetch the archive named in 'filename' from target over ssh, "
                   "verify its length matches 'size' and it holds the expected files"):
        if not room:
            print("Skipped, no room on the device")
        else:
            with open(local["big"], "wb") as f:
                tgtssh.run(f"sudo cat {remote}", check=True, stdout=f)
            if os.path.getsize(local["big"]) != int(output["size"]):
                raise Exception(f"RPC reported {output['size']} bytes,"
                                f" fetched {os.path.getsize(local['big'])}")
            verify_contents(local["big"], EXPECTED)

    test.succeed()
