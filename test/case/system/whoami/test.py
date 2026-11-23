#!/usr/bin/env python3
"""Who Am I

Verify the infix-system:whoami RPC.  It returns the name of the calling
user and the NACM groups that user is a member of.  The factory NACM
rules deny guests all RPCs but this one, since a guest cannot read the
NACM configuration to find out.
"""
import infamy

USER = "jane"
PASS = "guest123"
HASH = "$1$4rdUOhNN$vw3i4FyPvIkzRFwrUXQod1"


def whoami(target):
    """Call whoami, return username and the groups as a sorted list"""
    out = target.rpc_output("infix-system", "whoami")

    return out.get("username"), sorted(out.get("group", []))


with infamy.Test() as test:
    with test.step("Set up topology and attach to target DUT"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Add user jane to NACM groups guest and operator"):
        target.put_config_dicts({
            "ietf-system": {
                "system": {
                    "authentication": {
                        "user": [{
                            "name": USER,
                            "password": HASH,
                            "shell": "infix-system:bash"
                        }]
                    }
                }
            },
            "ietf-netconf-acm": {
                "nacm": {
                    "groups": {
                        "group": [{
                            "name": "guest",
                            "user-name": [USER]
                        }, {
                            "name": "operator",
                            "user-name": [USER]
                        }]
                    }
                }
            }
        })

    with test.step("Verify whoami as admin returns admin, in group admin"):
        user, groups = whoami(target)
        assert user == "admin", f"Unexpected username: {user}"
        assert groups == ["admin"], f"Unexpected groups: {groups}"

    with test.step("Verify whoami as jane returns jane, in groups guest and operator"):
        jane = env.attach("target", "mgmt", username=USER, password=PASS,
                          test_reset=False)
        user, groups = whoami(jane)
        assert user == USER, f"Unexpected username: {user}"
        assert groups == ["guest", "operator"], f"Unexpected groups: {groups}"

    with test.step("Verify jane cannot read the NACM groups"):
        cfg = jane.get_config_dict("/ietf-netconf-acm:nacm")
        assert not cfg, f"Guest can read the NACM configuration: {cfg}"

    test.succeed()
