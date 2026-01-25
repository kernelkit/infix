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


## TL;DR - The Factory Defaults Work

**You don't need to understand NACM to use the system securely.**

The factory configuration implements a "permit by default, deny sensitive
items" policy that provides:

- **Immediate usability** - Operators can configure the entire system
  without custom NACM rules
- **Automatic security** - Passwords, cryptographic keys, and dangerous
  operations are protected out-of-the-box
- **Future-proof design** - New features work immediately without NACM
  updates

The three built-in user levels (admin, operator, guest) cover most use
cases. Only read on if you need to create custom access control policies.


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
5. **YANG annotations override everything** - `nacm:default-deny-all` in a YANG module requires an explicit permit rule regardless of global defaults

### Example Rule Evaluation

Given this configuration:

```json
{
  "read-default": "permit",
  "write-default": "permit",
  "rule-list": [
    {
      "name": "operator-acl",
      "group": ["operator"],
      "rule": [
        {
          "name": "permit-system-rpcs",
          "module-name": "ietf-system",
          "access-operations": "exec",
          "action": "permit"
        }
      ]
    },
    {
      "name": "default-deny-all",
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

When operator user "jacky" tries to:

- **Read password**: Matches "deny-passwords" → **DENIED**
- **Write interface config**: No match, uses write-default → **PERMITTED** (write-default=permit)
- **Write hostname**: No match, uses write-default → **PERMITTED** (write-default=permit)
- **Reboot system**: Matches "permit-system-rpcs" → **PERMITTED** (overrides `nacm:default-deny-all`)

## Global Defaults

NACM has three global defaults that apply when no rule matches:

```json
{
  "read-default": "permit",
  "write-default": "permit",
  "exec-default": "permit"
}
```

**Factory configuration defaults:**

- `read-default: "permit"` - Users can read configuration and state by default
- `write-default: "permit"` - Users can modify configuration by default
- `exec-default: "permit"` - Users can execute RPCs by default

This permit-by-default approach, combined with targeted denials for
sensitive items (passwords, cryptographic keys), provides a balance
between usability and security. It also makes the system "future proof" -
when new YANG modules are added, operators can immediately configure them
without updating NACM rules.

> [!IMPORTANT]
> YANG modules with `nacm:default-deny-all` or `nacm:default-deny-write`
> annotations override these global defaults. You must create explicit
> permit rules for those operations.

## Module-Name vs Path

NACM rules can match operations using either `module-name` or `path`.
The following sub-sections provide detailed information and examples of
both.

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
- `/interfaces/interface/bridge-port` (from infix-interfaces)

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

> [!TIP] Use path-based rules
> When you want to permit/deny access to a configuration subtree
> including all augments, e.g., all IP settings below
> `/ietf-interfaces:interfaces/`.  This is more flexible and requires
> less maintenance as new features are added.

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

- `/system/authentication` (`nacm:default-deny-write`, [ietf-system][3])
- `/nacm` (`nacm:default-deny-all`, [RFC 8341][1])
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

The factory configuration uses a "permit by default, deny sensitive items"
approach. This design is "future proof" - when new YANG modules are added,
operators can immediately configure them without updating NACM rules.

```json
{
  "ietf-netconf-acm:nacm": {
    "enable-nacm": true,
    "read-default": "permit",
    "write-default": "permit",
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
            "action": "permit",
            "comment": "Admin has full unrestricted access"
          }
        ]
      },
      {
        "name": "operator-acl",
        "group": ["operator"],
        "rule": [
          {
            "name": "permit-system-rpcs",
            "module-name": "ietf-system",
            "rpc-name": "*",
            "access-operations": "exec",
            "action": "permit",
            "comment": "Operators can reboot, shutdown, and set system time"
          }
        ]
      },
      {
        "name": "guest-acl",
        "group": ["guest"],
        "rule": [
          {
            "name": "deny-all-write+exec",
            "module-name": "*",
            "access-operations": "create update delete exec",
            "action": "deny",
            "comment": "Guests can only read, not modify or execute"
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
            "action": "deny",
            "comment": "No user except admins can access password hashes"
          },
          {
            "name": "deny-keystore-access",
            "module-name": "ietf-keystore",
            "access-operations": "*",
            "action": "deny",
            "comment": "No user except admins can access cryptographic keys"
          },
          {
            "name": "deny-truststore-access",
            "module-name": "ietf-truststore",
            "access-operations": "*",
            "action": "deny",
            "comment": "No user except admins can access trust store"
          }
        ]
      }
    ]
  }
}
```

**Key design decisions:**

1. **Permit by default** - `write-default: permit` allows operators to configure any module
2. **Minimal operator rules** - Only one rule to permit system RPCs (reboot, set time)
3. **Future proof** - New YANG modules automatically configurable by operators
4. **Targeted denials** - Only sensitive items (passwords, keys) are explicitly denied
5. **Global denials** - Password/keystore/truststore denied for everyone via group "*"
6. **YANG annotations** - Sensitive operations (factory-reset, software upgrades) still protected by `nacm:default-deny-all` in YANG modules

**Effective permissions by group:**

| Group    | Read | Write | Exec | Exceptions                                    |
|----------|------|-------|------|-----------------------------------------------|
| admin    | All  | All   | All  | None                                          |
| operator | All  | All   | All  | Cannot access passwords, keystore, truststore |
| guest    | All  | None  | None | Read-only access                              |

## Common Patterns

### Permit-by-Default

The factory default approach - allow everything except sensitive items:

```json
{
  "write-default": "permit",
  "exec-default": "permit",
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
      "name": "global-denials",
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

This approach is "future proof" - new YANG modules are automatically
accessible without rule updates.  Admins bypass the global denials
because their `permit-all` rule is evaluated first.

### Deny-by-Default

More restrictive approach - deny everything except what is explicitly
allowed:

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

> [!NOTE]
> This requires updating NACM rules whenever new features are added.

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

<pre class="cli"><code>admin@example:/> <b>show nacm</b>
enabled              : yes
default read access  : permit
default write access : permit
default exec access  : permit
denied operations    : 0
denied data writes   : 0
denied notifications : 0

          ┌──────────┬─────────┬─────────┬─────────┐
          │ GROUP    │  READ   │  WRITE  │  EXEC   │
          ├──────────┼─────────┼─────────┼─────────┤
          │ admin    │    ✓    │    ✓    │    ✓    │
          │ operator │    ⚠    │    ⚠    │    ⚠    │
          │ guest    │    ⚠    │    ✗    │    ✗    │
          └──────────┴─────────┴─────────┴─────────┘
              ✓ Full    ⚠ Restricted    ✗ Denied

<span class="header">USER                   SHELL   LOGIN                            </span>
admin                  bash    password+key
jacky                  bash    password
monitor                false   key

<span class="header">GROUP                  USERS                                    </span>
admin                  admin
operator               jacky
guest                  monitor
</code></pre>

For details about a group's restrictions, use `show nacm group <name>`:

<pre class="cli"><code>admin@example:/> <b>show nacm group operator</b>
members          : jacky
read permission  : restricted
write permission : restricted
exec permission  : restricted
applicable rules : 4
──────────────────────────────────────────────────────────────────────
<span class="title">permit-system-rpcs</span>
  action     : permit
  operations : exec
  target     : ietf-system (rpc: *)

──────────────────────────────────────────────────────────────────────
<span class="title">deny-password-access (via '*')</span>
  action     : deny
  operations : *
  target     : /ietf-system:system/authentication/user/password

──────────────────────────────────────────────────────────────────────
<span class="title">deny-keystore-access (via '*')</span>
  action     : deny
  operations : *
  target     : ietf-keystore

──────────────────────────────────────────────────────────────────────
<span class="title">deny-truststore-access (via '*')</span>
  action     : deny
  operations : *
  target     : ietf-truststore
</code></pre>

### Testing Access

The easiest way to test NACM permissions is to log in as the user and try
the operation:

<pre class="cli"><code>$ ssh jacky@host
jacky@example:/> <b>configure</b>
jacky@example:/config/> <b>edit system authentication user admin</b>
jacky@example:/config/system/authentication/user/admin/> <b>set authorized-key foo</b>
Error: Access to the data model "ietf-system" is denied because "jacky" NACM authorization failed.
Error: Failed applying changes (2).
</code></pre>

### NACM Statistics

NACM tracks denied operations.  If you suspect permission issues, check
the statistics:

<pre class="cli"><code>admin@example:/> <b>show nacm</b>
...
  denied operations      : 5
  denied data writes     : 12
...
</code></pre>

Increasing counters indicate permission denials are occurring.

## Best Practices

1. **Leverage permit-by-default** - The factory configuration uses
   `write-default: "permit"` with targeted denials. This is "future proof" -
   new features work immediately without NACM updates.

2. **Protect sensitive items globally** - Use `group: ["*"]` rule-list to
   deny access to passwords, cryptographic keys, and similar sensitive data.
   Admin's permit-all rule (evaluated first) bypasses these denials.

3. **Leverage YANG annotations** - Many sensitive operations are already
   protected by `nacm:default-deny-all` in YANG modules (factory-reset,
   software upgrades, etc.). Only add explicit permit rules when needed.

4. **Order matters** - Rule-lists are evaluated in order. Place admin's
   permit-all rule first so it bypasses global denials.

5. **Use path-based denials** - For protecting specific data (like password
   hashes), use path rules. For protecting entire modules (like keystore),
   use module-name rules.

6. **Test thoroughly** - Always test user permissions after changes. NACM
   errors can be subtle (nodes may be silently omitted from read operations).

7. **Keep it simple** - The factory configuration uses only 6 rules for
   three user levels. Fewer rules are easier to understand and maintain.

8. **Document rules** - Use the "comment" field to explain why specific
   permissions are granted or denied.

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
