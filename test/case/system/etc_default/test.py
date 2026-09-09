#!/usr/bin/env python3
"""Daemon default files (/etc/default)

Verify that files stored in the configuration, under
/system/advanced/defaults, are written to /etc/default, that a file
shadowing one shipped with the system is restored when the entry is
disabled, and that a file is removed when the entry is deleted.
"""
import base64

import infamy
from infamy.util import parallel

NEW = "infamy-test"
SHADOW = "chronyd"


def content(text):
    return base64.b64encode(text.encode()).decode()


def cat(tgtssh, name):
    return tgtssh.runsh(f"cat /etc/default/{name}").stdout


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target, tgtssh = parallel(lambda: env.attach("target", "mgmt"),
                                  lambda: env.attach("target", "mgmt", "ssh"))

    with test.step("Record original /etc/default/chronyd"):
        original = cat(tgtssh, SHADOW)
        assert original, f"/etc/default/{SHADOW} missing on target"

    with test.step("Configure a new file and one shadowing chronyd"):
        target.put_config_dicts({
            "ietf-system": {
                "system": {
                    "infix-system:advanced": {
                        "defaults": {
                            "default": [
                                {
                                    "name": NEW,
                                    "content": content("INFAMY_ARGS=\"--test\"\n"),
                                },
                                {
                                    "name": SHADOW,
                                    "description": "Infamy shadow test",
                                    "content": content(original + "INFAMY_SHADOW=1\n"),
                                },
                            ],
                        }
                    }
                }
            }
        })

    with test.step("Verify both files are installed in /etc/default"):
        assert cat(tgtssh, NEW) == "INFAMY_ARGS=\"--test\"\n"
        assert cat(tgtssh, SHADOW).endswith("INFAMY_SHADOW=1\n")

    with test.step("Disable the shadowing entry"):
        target.put_config_dicts({
            "ietf-system": {
                "system": {
                    "infix-system:advanced": {
                        "defaults": {
                            "default": [{"name": SHADOW, "enabled": False}],
                        }
                    }
                }
            }
        })

    with test.step("Verify original /etc/default/chronyd is restored"):
        assert cat(tgtssh, SHADOW) == original

    with test.step("Delete the new file entry"):
        target.delete_xpath(f"/ietf-system:system/infix-system:advanced/defaults/default[name='{NEW}']")

    with test.step("Verify /etc/default/infamy-test is removed"):
        assert tgtssh.runsh(f"test -e /etc/default/{NEW}").returncode != 0

    test.succeed()
