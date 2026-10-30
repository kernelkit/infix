# Network Access Control Model (NACM)

The NETCONF Access Control Model ([RFC 8341][1]) controls who can read,
write, and execute operations on specific parts of the configuration and
operational state.  This document describes how rules are evaluated and
how to write access control policies of your own.

> [!TIP]
> For a practical introduction to user management and the built-in user
> levels (admin, operator, guest), see the [Multiple Users][2] section
> in the System Configuration guide.

## The Factory Defaults Work

You do not need to understand NACM to use the system securely.  The
factory configuration lets everyone read, except passwords and
cryptographic keys, but lets nobody change anything or run operations
unless their group is given that right.  A user that is not in any
group can look but not touch, and a new feature stays admin-only until
a rule gives it to someone else.

The three built-in user levels (admin, operator, guest) cover most
needs.  Read on only if you need to write access control policies of
your own.

## Overview

NACM controls three things: read and write access to data nodes,
execution of RPCs (remote procedure calls), and subscription to event
notifications.  Notifications are not covered here.

Three mechanisms decide the outcome of an access check, and they are
covered in turn below:

1. Global defaults for read, write, and exec
2. Annotations in the YANG modules themselves
3. Explicit permit and deny rules, organized in rule-lists

## Rule Evaluation

Rule-lists are processed in the order they appear in the configuration,
and within each rule-list the rules are tried in order.  The first
matching rule wins and no further rules are evaluated.  If no rule
matches, the global defaults apply.

The one exception is a node annotated with `nacm:default-deny-all` or
`nacm:default-deny-write` in its YANG module.  Such a node always
requires an explicit permit rule, whatever the global defaults say.

### Example Rule Evaluation

Given this configuration:

```json
{
  "read-default": "permit",
  "write-default": "deny",
  "exec-default": "deny",
  "rule-list": [
    {
      "name": "operator-acl",
      "group": ["operator"],
      "rule": [
        {
          "name": "permit-system-rpcs",
          "module-name": "ietf-system",
          "rpc-name": "*",
          "access-operations": "exec",
          "action": "permit"
        },
        {
          "name": "permit-interfaces",
          "path": "/ietf-interfaces:interfaces",
          "access-operations": "create update delete",
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

The operator user "jacky" gets the following results:

| Operation              | Matching rule        | Result                                     |
|------------------------|----------------------|--------------------------------------------|
| Read a password        | `deny-passwords`     | Denied                                     |
| Read interface config  | none                 | Permitted by `read-default`                |
| Write interface config | `permit-interfaces`  | Permitted                                  |
| Add a user account     | none                 | Denied by `write-default`                  |
| Reboot the system      | `permit-system-rpcs` | Permitted, despite `nacm:default-deny-all` |

## Global Defaults

NACM has three global defaults that apply when no rule matches:

```json
{
  "read-default": "permit",
  "write-default": "deny",
  "exec-default": "deny"
}
```

These are also the factory settings: any user may read what no rule or
YANG annotation denies, but changing anything or running an operation
takes a permit rule.  A user that is not in any group, or whose group
has no rules, therefore has read-only access.

> [!NOTE]
> Systems upgraded from a release where the factory settings were
> `"permit"` keep their saved defaults.  To get the same protection as a
> new installation, set `write-default` and `exec-default` to `"deny"`
> and add the operator rules shown in
> [Example: Factory Configuration](#example-factory-configuration).

> [!IMPORTANT]
> Nodes annotated with `nacm:default-deny-all` or `nacm:default-deny-write`
> in their YANG module ignore these defaults.  They need an explicit
> permit rule.

## Module-Name vs Path

A rule selects its target with either `module-name` or `path`.  The
difference matters for augmented modules.

### Module-Name Matching

Matches all nodes defined in one YANG module:

```json
{
  "name": "permit-keystore",
  "module-name": "ietf-keystore",
  "access-operations": "*",
  "action": "permit"
}
```

This permits all operations on data defined in ietf-keystore, but not on
nodes that other modules augment into it.  For example, the interface
list is defined in ietf-interfaces, while its IPv4 settings come from
ietf-ip, which augments it.  A rule naming ietf-interfaces therefore
does not cover `/interfaces/interface/ipv4/address`.

### Path Matching

Matches a subtree of the data tree, including nodes augmented into it
from other modules:

```json
{
  "name": "permit-network-config",
  "path": "/ietf-interfaces:interfaces/interface",
  "access-operations": "*",
  "action": "permit"
}
```

This permits operations on `/interfaces/interface` and everything below
it, including the `ipv4` and `ipv6` containers from ietf-ip and
`bridge-port` from infix-interfaces.

The path is written differently depending on whether the rule also has
a `module-name`.  With one, the path is relative to that module and
carries no prefix:

```json
"module-name": "ietf-system",
"path": "/system/authentication/user/password"
```

Without one, the path must start with the module prefix:

```json
"path": "/ietf-system:system/authentication/user/password"
```

> [!TIP] Use path-based rules
> To permit or deny a configuration subtree including its augments, e.g.,
> all IP settings below `/ietf-interfaces:interfaces/`, use a path rule.
> It keeps working as new augments are added.

## YANG-Level Annotations

Many YANG modules mark their sensitive nodes with NACM annotations, so a
baseline of protection exists before any rule is written.

### nacm:default-deny-all

Requires an explicit permit rule, whatever the global defaults say:

```yang
rpc system-restart {
  nacm:default-deny-all;
  description "Restart the system";
}
```

Even with `exec-default: "permit"`, users need an explicit permit rule to
execute system-restart.  It works the other way too: an explicit permit
rule, like the admin `permit-all`, opens up an annotated node.

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

The following RPCs are annotated and need an explicit permit:

- `ietf-factory-default:factory-reset` ([RFC 8808][4])
- `infix-factory-default:factory-default`
- `ietf-system:system-restart` ([ietf-system][3])
- `ietf-system:system-shutdown` ([ietf-system][3])
- `ietf-system:set-current-datetime` ([ietf-system][3])
- `infix-system:support-collect`
- `infix-system-software:install-bundle`
- `infix-system-software:set-boot-order`
- `infix-syslog:log`

So do these data containers:

- `/system/authentication` (`nacm:default-deny-write`, [ietf-system][3])
- `/system/advanced` (`nacm:default-deny-write`, boot scripts and daemon
  defaults run as root)
- `/nacm` (`nacm:default-deny-all`, [RFC 8341][1])
- Routing protocol key chains ([ietf-key-chain][5])
- RADIUS shared secrets ([ietf-system][3])
- TLS client/server credentials ([ietf-tls-client][6])

A mistake in the NACM rules therefore cannot open up these operations
by accident.

## Access Operations

NACM supports the following access operations:

- `create` - Create new data nodes
- `read` - Read existing data nodes
- `update` - Modify existing data nodes
- `delete` - Delete data nodes
- `exec` - Execute RPC operations
- `*` - All operations (wildcard)

A rule may list several, e.g., `"create update delete"` for all write
operations.

## Rule-List Groups

Each rule-list applies to one or more user groups:

```json
{
  "name": "operator-acl",
  "group": ["operator"],
  "rule": [...]
}
```

The group `"*"` matches all users, including those not in any NACM
group.  A user can be in several groups, in which case every rule-list
for any of those groups is evaluated, in configuration order, until a
rule matches.

## Example: Factory Configuration

The factory configuration denies write and exec by default, and has
five rule-lists, evaluated in this order:

| Rule-list                       | Group      | Purpose                                        |
|---------------------------------|------------|------------------------------------------------|
| `admin-acl`                     | `admin`    | `permit-all`, full access                      |
| `default-permit-read-only-exec` | `*`        | Permit operations that only read               |
| `operator-acl`                  | `operator` | Permit configuring the network, and reboot     |
| `guest-acl`                     | `guest`    | Deny all writes and operations                 |
| `default-deny-all`              | `*`        | Deny password hashes, keystore, and truststore |

The admin rule-list comes first, so its `permit-all` rule wins before
the global denials in the `"*"` rule-list are ever reached.  The
read-only operations come next, so the guest deny that follows does
not block them.

The operator rule-list permits the ietf-system RPCs (reboot, shutdown,
and setting the time), and writes to a few subtrees, each with a path
rule:

| Rule                 | Path                             |
|----------------------|----------------------------------|
| `permit-interfaces`  | `/ietf-interfaces:interfaces`    |
| `permit-routing`     | `/ietf-routing:routing`          |
| `permit-firewall`    | `/infix-firewall:firewall`       |
| `permit-dhcp-server` | `/infix-dhcp-server:dhcp-server` |
| `permit-lldp`        | `/ieee802-dot1ab-lldp:lldp`      |
| `permit-hostname`    | `/ietf-system:system/hostname`   |
| `permit-contact`     | `/ietf-system:system/contact`    |
| `permit-location`    | `/ietf-system:system/location`  |
| `permit-clock`       | `/ietf-system:system/clock`      |

Path rules leave the rest of a module alone, so the operator can set
the hostname, but not touch `/system/authentication` next to it.  A
`module-name` rule for ietf-system would have opened up the user
accounts as well, it overrides their YANG annotation.

A write rule does not cover the actions in a subtree, those are exec
operations with rules of their own.  Operators may run the actions
that go with the subtrees they configure:

| Rule                        | Action                                         |
|-----------------------------|------------------------------------------------|
| `permit-dhcp-server-clear`  | Clear the DHCP server statistics               |
| `permit-address-set-add`    | Add a dynamic entry to a firewall address set  |
| `permit-address-set-remove` | Remove a dynamic entry from an address set     |
| `permit-address-set-flush`  | Flush the dynamic entries of an address set    |

The firewall's `lockdown-mode` action is left out on purpose.  It cuts
all remote access, including the operator's own session, and only the
console can bring it back, so it stays with the admin.

Last, the operator rule-list permits the NETCONF operations that change
or save configuration: `edit-config`, `edit-data`, `lock`, `unlock`,
`commit`, `discard-changes`, `validate`, and `copy-config`.  These only
open up the operation, the write rules still decide which data it may
change.  The CLI, RESTCONF, and the web interface change and save
configuration without any operation, and do not need them.

The guest rule-list denies all writes and operations, which the
defaults already do, so guests stay read-only even if the defaults are
changed.

Some operations only read: NETCONF clients fetch the YANG modules they
need with `get-schema`, and a few show commands run an action, like
`show bridge fdb`, or `show hardware <radio> scan` and `survey`.  These
are exec operations, so the last rule-list permits them to every user:

| Rule                  | Operation                                    |
|-----------------------|----------------------------------------------|
| `permit-get-schema`   | `get-schema`                                 |
| `permit-bridge-fdb`   | the bridge `get-forwarding-database` action  |
| `permit-radio-scan`   | the Wi-Fi radio `scan` action                |
| `permit-radio-survey` | the Wi-Fi radio `channel-survey` action      |

The `get`, `get-config`, and `get-data` operations need no rule, NACM
never checks them for exec ([RFC 8341][1], section 3.2.4), and what
their reply holds is decided by the read rules.

Factory reset and software upgrade are not mentioned at all, their YANG
annotations, and the deny defaults, keep them admin-only.

The resulting permissions per group:

| Group    | Read | Write              | Exec                                                        | Exceptions                                    |
|----------|------|--------------------|-------------------------------------------------------------|-----------------------------------------------|
| admin    | All  | All                | All                                                         | None                                          |
| operator | All  | The subtrees above | Reboot, shutdown, set time, the operations above, read-only | Cannot access passwords, keystore, truststore |
| guest    | All  | None               | Read-only                                                   | Cannot access passwords, keystore, truststore |
| no group | All  | None               | Read-only                                                   | Cannot access passwords, keystore, truststore |

## Common Patterns

### Deny-by-Default

The factory approach, deny writes and operations unless a rule permits
them:

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

A user that is not in any group, or a group that is forgotten, gets
read-only access.

> [!NOTE]
> New YANG modules need a permit rule before such a group can change
> them.

### Permit-by-Default

Allow everything except the sensitive items:

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

> [!WARNING]
> Every user, including one in no group, can then change everything that
> is not denied or annotated in YANG, also the features of future
> releases.  Annotations cannot be added to standard IETF modules, so
> some of their sensitive nodes may stay open.

### Global Restrictions

Deny sensitive data for all users, except admins whose `permit-all` rule
is evaluated first:

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

`show nacm` lists the defaults, the groups, and which users are in them:

<pre class="cli"><code>admin@example:/> <b>show nacm</b>
enabled              : yes
default read access  : permit
default write access : deny
default exec access  : deny
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
applicable rules : 29
──────────────────────────────────────────────────────────────────────
<span class="title">permit-system-rpcs</span>
  action     : permit
  operations : exec
  target     : ietf-system (rpc: *)

──────────────────────────────────────────────────────────────────────
<span class="title">permit-interfaces</span>
  action     : permit
  operations : create update delete
  target     : /ietf-interfaces:interfaces
...
</code></pre>

### Testing Access

The easiest way to test NACM permissions is to log in as the user and try
the operation:

<pre class="cli"><code>$ <b>ssh jacky@host</b>
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

Counters that keep increasing mean some user is being denied.

## Best Practices

Start from the factory configuration rather than from scratch.  Keep the
deny defaults and give each group the rights it needs, rather than
permitting everything and denying piece by piece.  A forgotten rule then
leaves a user with too little access, not too much.

Put the admin rule-list first.  Rule-lists are evaluated in order, and
the `permit-all` rule has to win before any global denial is reached.
Then put the denials that apply to everyone else in a `"*"` rule-list.

Permit by path rather than by module.  A permit rule overrides the YANG
annotations, so a `module-name` rule for ietf-system would also open up
the user accounts, while a path rule for the hostname does not.

Test as the affected user after every change.  A denied read does not
produce an error, the node is simply left out of the reply, so a rule
mistake is easy to miss.  Give every rule a `comment` saying why it
exists.

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
