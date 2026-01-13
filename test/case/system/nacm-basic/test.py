#!/usr/bin/env python3
"""
Verify basic NACM permission enforcement

Test that NACM groups (admin, operator, guest) correctly enforce
access control with permissive defaults and targeted denials.

The NACM design is "permit by default, deny sensitive items":

- admin: Full unrestricted access (permit-all rule)
- operator: Can configure everything EXCEPT passwords, keystore, truststore
- guest: Read-only access (explicit deny of create/update/delete/exec)

Verifies that:

- Operators can read and modify most configuration (hostname, interfaces)
- Operators CANNOT read or write password hashes (protected path)
- Guests can read but cannot modify any configuration
- Admin can access everything including passwords
"""
import infamy

OPERATOR_USER = "oper"
OPERATOR_PASS = "oper123"
GUEST_USER = "guest"
GUEST_PASS = "guest123"

with infamy.Test() as test:
    with test.step("Set up topology and attach to target"):
        env = infamy.Env()
        target = env.attach("target", "mgmt")

    with test.step("Configure NACM groups, rules, and test users"):
        operator_hash = "$1$gwU5mRgP$1/ASdRwD5ycqdmWpTKHSa0"
        guest_hash = "$1$4rdUOhNN$vw3i4FyPvIkzRFwrUXQod1"

        target.put_config_dicts({
            "ietf-system": {
                "system": {
                    "authentication": {
                        "user": [{
                            "name": OPERATOR_USER,
                            "password": operator_hash,
                            "shell": "infix-system:bash"
                        }, {
                            "name": GUEST_USER,
                            "password": guest_hash,
                            "shell": "infix-system:bash"
                        }]
                    }
                }
            },
            "ietf-netconf-acm": {
                "nacm": {
                    "enable-nacm": True,
                    "read-default": "permit",
                    "write-default": "permit",
                    "exec-default": "permit",
                    "groups": {
                        "group": [{
                            "name": "admin",
                            "user-name": ["admin"]
                        }, {
                            "name": "operator",
                            "user-name": [OPERATOR_USER]
                        }, {
                            "name": "guest",
                            "user-name": [GUEST_USER]
                        }]
                    },
                    "rule-list": [{
                        "name": "admin-acl",
                        "group": ["admin"],
                        "rule": [{
                            "name": "permit-all",
                            "module-name": "*",
                            "access-operations": "*",
                            "action": "permit",
                            "comment": "Admin has full access"
                        }]
                    }, {
                        "name": "operator-acl",
                        "group": ["operator"],
                        "rule": [{
                            "name": "permit-system-rpcs",
                            "module-name": "ietf-system",
                            "access-operations": "exec",
                            "action": "permit",
                            "comment": "Operators can reboot, shutdown, and set system time"
                        }]
                    }, {
                        "name": "guest-acl",
                        "group": ["guest"],
                        "rule": [{
                            "name": "deny-all-exec",
                            "module-name": "*",
                            "access-operations": "create update delete exec",
                            "action": "deny",
                            "comment": "Guests cannot change anything or call rpcs"
                        }]
                    }, {
                        "name": "default-deny-all",
                        "group": ["*"],
                        "rule": [{
                            "name": "deny-password-access",
                            "path": "/ietf-system:system/authentication/user/password",
                            "access-operations": "*",
                            "action": "deny",
                            "comment": "No user except admins can access password hashes."
                        }, {
                            "name": "deny-keystore-access",
                            "module-name": "ietf-keystore",
                            "access-operations": "*",
                            "action": "deny",
                            "comment": "No user except admins can access cryptographic keys."
                        }, {
                            "name": "deny-truststore-access",
                            "module-name": "ietf-truststore",
                            "access-operations": "*",
                            "action": "deny",
                            "comment": "No user except admins can access trust store."
                        }]
                    }]
                }
            }
        })

    with test.step("Verify operator can read configuration"):
        # Attach as operator user
        operator = env.attach("target", "mgmt", username=OPERATOR_USER,
                              password=OPERATOR_PASS, test_reset=False)

        # Operator should be able to read (read-default: permit)
        ifaces = operator.get_config_dict("/ietf-interfaces:interfaces")
        num_ifaces = len(ifaces.get('interfaces', {}).get('interface', []))
        print(f"Operator successfully read {num_ifaces} interfaces")

    with test.step("Verify operator can modify interface configuration"):
        # Use patch_config() which PATCHes directly to running datastore
        # This avoids full datastore copy that requires broader permissions
        operator.patch_config("ietf-interfaces", {
            "interfaces": {
                "interface": [{
                    "name": "lo",
                    "description": "Modified by operator"
                }]
            }
        })

        # Verify the change
        ifaces = operator.get_config_dict("/ietf-interfaces:interfaces")
        lo_iface = None
        for iface in ifaces.get('interfaces', {}).get('interface', []):
            if iface.get('name') == 'lo':
                lo_iface = iface
                break
        assert lo_iface and lo_iface.get('description') == "Modified by operator", \
            "Operator failed to modify interface"
        print("Operator successfully modified interface configuration")

    with test.step("Verify operator can modify hostname"):
        # Operators can now modify most system config (write-default: permit)
        operator.patch_config("ietf-system", {
            "system": {
                "hostname": "operator-test"
            }
        })

        # Verify the change
        cfg = operator.get_config_dict("/ietf-system:system")
        assert cfg.get("system", {}).get("hostname") == "operator-test", \
            "Operator failed to modify hostname"
        print("Operator successfully modified hostname")

    with test.step("Verify operator cannot read password hashes"):
        # Password hashes are protected by deny-password-access rule
        cfg = operator.get_config_dict("/ietf-system:system")
        users = cfg.get("system", {}).get("authentication", {}).get("user", [])

        # Check that no user entry contains a password field
        for user in users:
            assert "password" not in user, \
                f"Operator should NOT be able to read password for user '{user.get('name')}'"
        print("Operator correctly denied read access to password hashes")

    with test.step("Verify operator cannot write password hashes"):
        # Try to change a password - should fail due to NACM deny rule
        # Use a valid MD5 hash format to ensure we test NACM, not YANG validation
        valid_hash = "$1$testsalt$YvPTBnV5RhkWwXLzR7kK/1"
        try:
            operator.patch_config("ietf-system", {
                "system": {
                    "authentication": {
                        "user": [{
                            "name": OPERATOR_USER,
                            "password": valid_hash
                        }]
                    }
                }
            }, retries=1)
            assert False, "Operator should NOT be able to modify passwords!"
        except Exception as e:
            error_msg = str(e)
            assert any(keyword in error_msg for keyword in ["403", "Forbidden", "denied", "authorization failed"]), \
                f"Expected NACM denial, got: {e}"
            print("Operator correctly denied write access to password hashes")

    with test.step("Verify guest can read configuration"):
        # Attach as guest user
        guest = env.attach("target", "mgmt", username=GUEST_USER,
                           password=GUEST_PASS, test_reset=False)

        # Guest should be able to read (read-default: permit)
        ifaces = guest.get_config_dict("/ietf-interfaces:interfaces")
        num_ifaces = len(ifaces.get('interfaces', {}).get('interface', []))
        print(f"Guest successfully read {num_ifaces} interfaces")

    with test.step("Verify guest cannot modify configuration"):
        # Try to modify hostname - should fail (write-default: deny)
        # Use patch_config with retries=1 since NACM denials won't succeed on retry
        try:
            guest.patch_config("ietf-system", {
                "system": {
                    "hostname": "hacked"
                }
            }, retries=1)
            assert False, "Guest should NOT be able to modify configuration!"
        except Exception as e:
            error_msg = str(e)
            assert any(keyword in error_msg for keyword in ["403", "Forbidden", "denied", "authorization failed"]), \
                f"Expected NACM denial, got: {e}"
            print("Guest correctly denied write access")

    with test.step("Verify admin can access passwords"):
        # Admin should have full access including protected paths
        cfg = target.get_config_dict("/ietf-system:system")
        users = cfg.get("system", {}).get("authentication", {}).get("user", [])

        # Admin should be able to see password hashes
        has_password = any("password" in user for user in users)
        assert has_password, "Admin should be able to read password hashes"
        print("Admin successfully read password hashes")

        # Admin can also modify passwords
        target.put_config_dicts({
            "ietf-system": {
                "system": {
                    "hostname": "admin-test"
                }
            }
        })
        cfg = target.get_config_dict("/ietf-system:system")
        assert cfg.get("system", {}).get("hostname") == "admin-test", \
            "Admin hostname change not applied"
        print("Admin successfully modified hostname")

    test.succeed()
