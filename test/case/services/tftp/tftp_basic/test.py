#!/usr/bin/env python3
"""TFTP Server

Verify that the TFTP server serves files uploaded to its root directory
with the copy command, and that the operational datastore lists them.
Files that are not world-readable must neither be served nor listed.

"""
import infamy
from infamy.util import parallel, until

ADDR = "10.0.0.10"
ROOT = "/var/lib/tftpboot"
PUBLIC = "public.bin"
PRIVATE = "private.bin"
CONTENT = "netboot fallback image"


def fetch(ns, name):
    """Fetch name over TFTP, return the file contents or None"""
    res = ns.run(["curl", "-s", "-m", "2", f"tftp://{ADDR}/{name}"],
                 capture_output=True, text=True)
    return res.stdout if res.returncode == 0 else None


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target, tgtssh = parallel(lambda: env.attach("target", "mgmt"),
                                  lambda: env.attach("target", "mgmt", "ssh"))
        _, hport = env.ltop.xlate("host", "data")
        test.push_test_cleanup(lambda: tgtssh.runsh(f"sudo rm -f {ROOT}/{PUBLIC} {ROOT}/{PRIVATE}"))

    with test.step("Configure target:data with 10.0.0.10/24 and enable TFTP server"):
        target.put_config_dicts({
            "ietf-interfaces": {
                "interfaces": {
                    "interface": [{
                        "name": target["data"],
                        "enabled": True,
                        "ipv4": {
                            "address": [{
                                "ip": ADDR,
                                "prefix-length": 24
                            }]
                        }
                    }]
                }
            },
            "infix-services": {
                "tftp": {
                    "enabled": True
                }
            }
        })

    with test.step("Upload a file to the TFTP root with copy, and place a private file beside it"):
        tgtssh.runsh(f"""
            printf "{CONTENT}" > /tmp/{PUBLIC}
            sudo copy -s -f /tmp/{PUBLIC} {ROOT}/{PUBLIC}
            sudo sh -c 'umask 077; printf secret > {ROOT}/{PRIVATE}'
        """)

    with test.step("Verify operational datastore lists only the uploaded file"):
        def listed():
            files = target.get_data("/infix-services:tftp")["tftp"]["files"]["file"]
            return {f["name"] for f in files} == {PUBLIC}
        until(listed)

    with infamy.IsolatedMacVlan(hport) as ns:
        with test.step("Verify the uploaded file is served over TFTP"):
            ns.addip("10.0.0.1")
            until(lambda: fetch(ns, PUBLIC) == CONTENT)

        with test.step("Verify the private file is refused"):
            if fetch(ns, PRIVATE) is not None:
                test.fail()

    test.succeed()
