#!/usr/bin/env python3
"""
Verify basic NACM permission enforcement

Test that NACM groups (admin, operator, guest) correctly enforce
access control with path-based rules and default policies.

Creates three user privilege levels from scratch:

- admin: Full access (permit-all rule)
- operator: Can manage interfaces/routing, cannot modify system config
- guest: Read-only access (write-default: deny, exec deny rule)

Verifies that:

- All users can read configuration (read-default: permit)
- Operators can modify interfaces (path-specific permit rule)
- Operators cannot modify system configuration (write-default: deny)
- Guests cannot modify any configuration (write-default: deny)
- Admin can modify all configuration (permit-all rule bypasses defaults)
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
                    "write-default": "deny",
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
                            "name": "permit-interfaces",
                            "path": "/ietf-interfaces:interfaces/interface",
                            "access-operations": "*",
                            "action": "permit",
                            "comment": "Operators can manage interfaces"
                        }, {
                            "name": "permit-routing",
                            "path": "/ietf-routing:routing",
                            "access-operations": "*",
                            "action": "permit",
                            "comment": "Operators can manage routing"
                        }, {
                            "name": "deny-users",
                            "path": "/ietf-system:system/authentication",
                            "access-operations": "*",
                            "action": "deny",
                            "comment": "Operators cannot manage users"
                        }]
                    }, {
                        "name": "guest-acl",
                        "group": ["guest"],
                        "rule": [{
                            "name": "deny-all-exec",
                            "module-name": "*",
                            "access-operations": "exec",
                            "action": "deny",
                            "comment": "Guests cannot execute operations"
                        }]
                    }, {
                        "name": "default-deny-passwords",
                        "group": ["*"],
                        "rule": [{
                            "name": "deny-password-access",
                            "path": "/ietf-system:system/authentication/user/password",
                            "access-operations": "*",
                            "action": "deny",
                            "comment": "No user can access password hashes"
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

    with test.step("Verify operator cannot modify system configuration"):
        # Try to modify system config - should fail (write-default: deny)
        # Use patch_config with retries=1 since NACM denials won't succeed on retry
        try:
            operator.patch_config("ietf-system", {
                "system": {
                    "hostname": "operator-test"
                }
            }, retries=1)
            assert False, "Operator should NOT be able to modify system config!"
        except Exception as e:
            error_msg = str(e)
            # Check for NACM denial (different error messages for RESTCONF vs NETCONF)
            assert any(keyword in error_msg for keyword in ["403", "Forbidden", "denied", "authorization failed"]), \
                f"Expected NACM denial, got: {e}"
            print("Operator correctly denied system config access")

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

    with test.step("Verify admin can modify configuration"):
        # Admin should have full access (permit-all rule)
        target.put_config_dicts({
            "ietf-system": {
                "system": {
                    "hostname": "admin-test"
                }
            }
        })
        # Verify the change
        cfg = target.get_config_dict("/ietf-system:system")
        assert cfg.get("system", {}).get("hostname") == "admin-test", \
            "Admin hostname change not applied"
        print("Admin successfully modified hostname")

    test.succeed()
