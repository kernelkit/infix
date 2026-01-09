# Network Access Control Model (NACM)

NETCONF Access Control Model ([RFC 8341][1]) provides fine-grained access
control for YANG data models. NACM controls who can read, write, and
execute operations on specific parts of the configuration and operational
state.

This document provides technical details about how NACM works, how rules
are evaluated, and best practices for creating custom access control
policies.

> [!TIP]
> For a practical introduction to user management and the built-in user
> levels (admin, operator, guest), see the [Multiple Users][2] section
> in the System Configuration guide.


## Overview

NACM provides three types of access control:

- **Data node access** - Control read/write access to configuration and state
- **Operation access** - Control execution of RPCs (remote procedure calls)
- **Notification access** - Control subscription to event notifications (not covered here)

Access is controlled through:

1. **Global defaults** - Default permissions for read/write/exec
2. **YANG-level annotations** - Security markers in YANG modules (see below)
3. **NACM rules** - Explicit permit/deny rules organized in rule-lists


## Rule Evaluation

NACM rules are evaluated in a specific order:

1. **Rule-lists are processed sequentially** in the order they appear in the configuration
2. **Within each rule-list**, rules are evaluated sequentially
3. **First matching rule wins** - no further rules are evaluated
4. **If no rule matches**, global defaults apply (read-default, write-default, exec-default)
5. **YANG annotations override everything** - nacm:default-deny-all in a YANG module requires an explicit permit rule regardless of global defaults

### Example Rule Evaluation

Given this configuration:

```json
{
  "read-default": "permit",
  "write-default": "deny",
  "rule-list": [
    {
      "name": "operator-acl",
      "group": ["operator"],
      "rule": [
        {
          "name": "deny-passwords",
          "path": "/ietf-system:system/authentication/user/password",
          "access-operations": "read",
          "action": "deny"
        },
        {
          "name": "permit-interfaces",
          "path": "/ietf-interfaces:interfaces/interface",
          "access-operations": "*",
          "action": "permit"
        }
      ]
    }
  ]
}
```

When operator user "jacky" tries to:

- **Read password**: Matches "deny-passwords" → **DENIED**
- **Write interface config**: Matches "permit-interfaces" → **PERMITTED**
- **Read routing config**: No match, uses write-default → **DENIED** (write-default=deny)
- **Read system state**: No match, uses read-default → **PERMITTED** (read-default=permit)


## Global Defaults

NACM has three global defaults that apply when no rule matches:

```json
{
  "read-default": "permit",
  "write-default": "deny",
  "exec-default": "permit"
}
```

**Factory configuration defaults:**
- `read-default: "permit"` - Users can read configuration and state by default
- `write-default: "deny"` - Users cannot modify configuration unless explicitly permitted
- `exec-default: "permit"` - Users can execute RPCs unless explicitly denied

This deny-by-default approach for writes provides good security while
allowing read access for monitoring and troubleshooting.

> [!IMPORTANT]
> YANG modules with `nacm:default-deny-all` or `nacm:default-deny-write`
> annotations override these global defaults. You must create explicit
> permit rules for those operations.

## Module-Name vs Path

NACM rules can match operations using either `module-name` or `path`:

### Module-Name Matching

Matches all nodes **defined** in a specific YANG module:

```json
{
  "name": "permit-keystore",
  "module-name": "ietf-keystore",
  "access-operations": "*",
  "action": "permit"
}
```

This permits all operations on data **defined** in ietf-keystore, but does
NOT cover augments from other modules.

**Example:** The `/interfaces/interface/ipv4/address` path:

- Interface is defined in `ietf-interfaces`
- IPv4 config is defined in `ietf-ip` (augments ietf-interfaces)
- A rule with `module-name: "ietf-interfaces"` does NOT cover ipv4/address

### Path Matching

Matches a specific data tree path and **all nodes under it**, including
augments from other modules:

```json
{
  "name": "permit-network-config",
  "path": "/ietf-interfaces:interfaces/interface",
  "access-operations": "*",
  "action": "permit"
}
```

This permits operations on `/interfaces/interface` **and all child nodes**,
including augments like:
- `/interfaces/interface/ipv4` (from ietf-ip)
- `/interfaces/interface/ipv6` (from ietf-ip)
- `/interfaces/interface/bridge-port` (from ieee802-dot1q-bridge)

**Path syntax:**
- When used **with** module-name: Path is module-relative (no prefix)
  ```json
  "module-name": "ietf-system",
  "path": "/system/authentication/user/password"
  ```

- When used **without** module-name: Path must include module prefix
  ```json
  "path": "/ietf-system:system/authentication/user/password"
  ```

> [!TIP]
> **Use path-based rules** when you want to permit/deny access to a
> configuration subtree including all augments. This is more flexible and
> requires less maintenance as new features are added.


## YANG-Level Annotations

Many YANG modules include NACM annotations that provide baseline security:

### nacm:default-deny-all

Requires an explicit permit rule, regardless of global defaults:

```yang
rpc system-restart {
  nacm:default-deny-all;
  description "Restart the system";
}
```

Even with `exec-default: "permit"`, users need an explicit permit rule to
execute system-restart.

### nacm:default-deny-write

Write operations require an explicit permit rule:

```yang
container authentication {
  nacm:default-deny-write;
  description "User authentication configuration";
}
```

Even with `write-default: "permit"`, users need an explicit permit rule to
modify authentication settings.

### Protected Operations

The following are protected by YANG annotations and require explicit permits:

**RPC Operations:**
- `ietf-system:system-restart` ([ietf-system][3])
- `ietf-system:system-shutdown` ([ietf-system][3])
- `ietf-system:set-current-datetime` ([ietf-system][3])
- `infix-factory-default:factory-default`
- `ietf-factory-default:factory-reset` ([RFC 8808][4])
- `infix-system-software:install-bundle`
- `infix-system-software:set-boot-order`

**Data Containers:**
- `/system/authentication` (nacm:default-deny-write, [ietf-system][3])
- `/nacm` (nacm:default-deny-all, [RFC 8341][1])
- Routing protocol key chains ([ietf-key-chain][5])
- RADIUS shared secrets ([ietf-system][3])
- TLS client/server credentials ([ietf-tls-client][6])

This provides defense-in-depth - even if NACM rules are misconfigured, these
critical operations remain protected.


## Access Operations

NACM supports the following access operations:

- `create` - Create new data nodes
- `read` - Read existing data nodes
- `update` - Modify existing data nodes
- `delete` - Delete data nodes
- `exec` - Execute RPC operations
- `*` - All operations (wildcard)

**Common combinations:**
- `"create update delete"` - All write operations
- `"*"` - Everything (read, write, execute)
- `"read"` - Read-only access


## Rule-List Groups

Each rule-list applies to one or more user groups:

```json
{
  "name": "operator-acl",
  "group": ["operator"],
  "rule": [...]
}
```

**Special group names:**
- `"*"` - Matches all users (including those not in any NACM group)

**Evaluation:**
A user can be in multiple NACM groups. All rule-lists matching the user's
groups are evaluated in order until a matching rule is found.


## Example: Factory Configuration

The factory configuration provides a minimal NACM setup using global
defaults and path-based rules:

```json
{
  "ietf-netconf-acm:nacm": {
    "enable-nacm": true,
    "read-default": "permit",
    "write-default": "deny",
    "exec-default": "permit",
    "groups": {
      "group": [
        {"name": "admin", "user-name": ["admin"]},
        {"name": "operator", "user-name": []},
        {"name": "guest", "user-name": []}
      ]
    },
    "rule-list": [
      {
        "name": "admin-acl",
        "group": ["admin"],
        "rule": [
          {
            "name": "permit-all",
            "module-name": "*",
            "access-operations": "*",
            "action": "permit"
          }
        ]
      },
      {
        "name": "operator-acl",
        "group": ["operator"],
        "rule": [
          {
            "name": "permit-interfaces",
            "path": "/ietf-interfaces:interfaces/interface",
            "access-operations": "*",
            "action": "permit"
          },
          {
            "name": "permit-routing",
            "path": "/ietf-routing:routing",
            "access-operations": "*",
            "action": "permit"
          },
          {
            "name": "permit-containers",
            "path": "/infix-containers:containers",
            "access-operations": "*",
            "action": "permit"
          },
          {
            "name": "permit-firewall",
            "path": "/infix-firewall:firewall",
            "access-operations": "*",
            "action": "permit"
          },
          {
            "name": "permit-system-restart",
            "module-name": "ietf-system",
            "rpc-name": "system-restart",
            "access-operations": "exec",
            "action": "permit"
          }
        ]
      },
      {
        "name": "guest-acl",
        "group": ["guest"],
        "rule": [
          {
            "name": "deny-all-exec",
            "module-name": "*",
            "access-operations": "exec",
            "action": "deny"
          }
        ]
      },
      {
        "name": "default-deny-all",
        "group": ["*"],
        "rule": [
          {
            "name": "deny-password-access",
            "path": "/ietf-system:system/authentication/user/password",
            "access-operations": "*",
            "action": "deny"
          },
          {
            "name": "deny-keystore-access",
            "module-name": "ietf-keystore",
            "access-operations": "*",
            "action": "deny"
          },
          {
            "name": "deny-truststore-access",
            "module-name": "ietf-truststore",
            "access-operations": "*",
            "action": "deny"
          }
        ]
      }
    ]
  }
}
```

**Key design decisions:**

1. **Minimal rules** - Only 10 rules total, leveraging global defaults and YANG annotations
2. **Path-based permits** - Operator rules use paths to cover containers and all augments
3. **Explicit RPC permit** - system-restart explicitly permitted for operators (has nacm:default-deny-all)
4. **Global denials** - Password/keystore/truststore denied for everyone via group "*"
5. **YANG annotations** - Most sensitive operations (factory-reset, software upgrades, etc.) protected by YANG-level nacm:default-deny-all


## Common Patterns

### Deny-by-Default with Explicit Permits

Most secure approach - deny everything except what's explicitly allowed:

```json
{
  "write-default": "deny",
  "exec-default": "deny",
  "rule-list": [
    {
      "group": ["limited-user"],
      "rule": [
        {
          "name": "permit-interface-config",
          "path": "/ietf-interfaces:interfaces/interface",
          "access-operations": "create update delete",
          "action": "permit"
        }
      ]
    }
  ]
}
```

### Permit-All with Specific Denials

More permissive - allow everything except sensitive operations:

```json
{
  "rule-list": [
    {
      "group": ["operator"],
      "rule": [
        {
          "name": "deny-user-management",
          "path": "/ietf-system:system/authentication",
          "access-operations": "create update delete",
          "action": "deny"
        },
        {
          "name": "permit-all",
          "module-name": "*",
          "access-operations": "*",
          "action": "permit"
        }
      ]
    }
  ]
}
```

> [!IMPORTANT]
> Specific denials must come **before** broad permits in the rule list,
> since first matching rule wins.

### Global Restrictions

Deny access to sensitive data for all users (except admins with permit-all):

```json
{
  "rule-list": [
    {
      "group": ["*"],
      "rule": [
        {
          "name": "deny-passwords",
          "path": "/ietf-system:system/authentication/user/password",
          "access-operations": "*",
          "action": "deny"
        }
      ]
    }
  ]
}
```


## Debugging NACM

### Viewing Effective Permissions

Check what NACM groups a user belongs to:

```
admin@example:/> show nacm
enabled                  : yes
default read access      : permit
default write access     : deny
default exec access      : permit
denied operations        : 0
denied data writes       : 0
denied notifications     : 0

USER                   SHELL   LOGIN
admin                  bash    password+key
jacky                  bash    password
monitor                false   key

GROUP                  USERS
admin                  admin
operator               jacky
guest                  monitor
```

### Testing Access

The easiest way to test NACM permissions is to log in as the user and try
the operation:

```bash
$ ssh jacky@host
jacky@example:/> configure
jacky@example:/config/> edit system authentication user admin
jacky@example:/config/system/authentication/user/admin/> set authorized-key foo
Error: Access to the data model "ietf-system" is denied because "jacky" NACM authorization failed.
Error: Failed applying changes (2).
```

### NACM Statistics

NACM tracks denied operations. If you suspect permission issues, check the
statistics:

```
admin@example:/> show nacm
...
  denied operations      : 5
  denied data writes     : 12
...
```

Increasing counters indicate permission denials are occurring.


## Best Practices

1. **Start with deny-by-default** - Use `write-default: "deny"` and
   `exec-default: "deny"` for new systems, then add permits as needed.

2. **Use path-based rules** - Prefer path over module-name for broader
   coverage including augments.

3. **Leverage YANG annotations** - Many sensitive operations are already
   protected by nacm:default-deny-all in YANG modules. Don't duplicate
   these as explicit NACM rules.

4. **Order matters** - Place specific denials before broad permits in each
   rule-list.

5. **Use global denials** - For restrictions that apply to everyone (except
   admins), use `group: ["*"]` rule-list.

6. **Test thoroughly** - Always test user permissions after changes. NACM
   errors can be subtle (nodes may be silently omitted from read operations).

7. **Keep it simple** - Fewer, broader rules are easier to maintain than
   many specific rules. The factory configuration uses only 10 rules for
   three user levels (1 admin, 5 operator, 1 guest, 3 global).

8. **Document exceptions** - Use the "comment" field in rules to explain
   why specific permissions are granted or denied.


## References

- [RFC 8341: Network Configuration Access Control Model (NACM)][1]
- [RFC 7317: A YANG Data Model for System Management (ietf-system)][3]
- [RFC 8808: A YANG Data Model for Factory Default Settings (ietf-factory-default)][4]
- [RFC 8177: YANG Key Chain (ietf-key-chain)][5]
- [System Configuration - Multiple Users][2]

[1]: https://www.rfc-editor.org/rfc/rfc8341
[2]: system.md#multiple-users
[3]: https://www.rfc-editor.org/rfc/rfc7317
[4]: https://www.rfc-editor.org/rfc/rfc8808
[5]: https://www.rfc-editor.org/rfc/rfc8177
[6]: https://datatracker.ietf.org/doc/html/draft-ietf-netconf-tls-client-server
