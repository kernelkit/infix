#!/usr/bin/env python3
"""Support data collection

Verify that the support-collect RPC returns a valid archive with the
expected content, that the archive can be GPG encrypted, and that an
archive too large to return inline is left on the device and its path
returned instead.

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


def free_kb(ssh, path):
    """Free space on the filesystem holding path, in KiB"""
    result = ssh.runsh(f"df -Pk {path} | awk 'NR == 2 {{ print $4 }}'")
    return int(result.stdout.strip())


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
        for name in ("archive", "encrypted", "decrypted", "big"):
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
            raise Exception("gpg is required on the test host")
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

    with test.step("Attach to target over ssh and create /var/log/support-test.bin "
                   "with 17 MB of random data"):
        tgtssh = env.attach("target", "mgmt", "ssh", test_reset=False)

        free = free_kb(tgtssh, WORK_DIR)
        need = 2 * (os.path.getsize(local["archive"]) // 1024 + BIG_MB * 1024) + 2048
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
            verify(local["big"], EXPECTED)

    test.succeed()
