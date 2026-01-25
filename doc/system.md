# System Configuration

System settings in Infix are provided by the [ietf-system][1] YANG
model, augmented with Linux specific extensions in [infix-system][2],
like Message of the Day (login message) and user login shell.  More
on this later on in this document.

For the sake of brevity, the hostname in the following examples has been
shortened to `example`.  The default hostname is composed from a product
specific string followed by the last three octets of the system base MAC
address, e.g., `switch-12-34-56`.  An example of how to change the
hostname is included below.

> [!NOTE]
> When issuing `leave` to activate your changes, remember to also save
> your settings, `copy running-config startup-config`.  See the [CLI
> Introduction](cli/introduction.md) for a background.


## Changing Password

User management, including passwords, SSH keys, remote authentication is
available in the system authentication configuration context.

<pre class="cli"><code>admin@example:/config/> <b>edit system authentication user admin</b>
admin@example:/config/system/…/user/admin/> <b>change password</b>
New password:
Retype password:
admin@example:/config/system/…/user/admin/> <b>leave</b>
</code></pre>

The `change password` command starts an interactive dialogue that asks
for the new password, with a confirmation, and then salts and encrypts
the password with sha512crypt.

It is also possible to use the `set password ...` command.  This allows
setting an already hashed password.  To manually hash a password, use
the `do password encrypt` command.  This launches the admin-exec command
to hash, and optionally salt, your password.  This encrypted string can
then be used with `set password ...`.

> [!TIP]
> If you are having trouble thinking of a password, there is a nifty
> `password generate` command in admin-exec context which generates
> random passwords using the UNIX command `pwgen`.  Use the `do` prefix
> when inside any configuration context to access admin-exec commands.


### SSH Authorized Key

Logging in remotely with SSH is possible by adding a *public key* to a
user.  Here we add the authorized key to the admin user, multiple keys
are supported.

With SSH keys in place it is possible to disable password login, just
remember to verify SSH login and network connectivity before doing so.

<pre class="cli"><code>admin@example:/config/> <b>edit system authentication user admin</b>
admin@example:/config/system/…/user/admin/> <b>edit authorized-key admin@example</b>
admin@example:/config/system/…/example@host/> <b>set algorithm ssh-rsa</b>
admin@example:/config/system/…/example@host/> <b>set key-data AAAAB3NzaC1yc2EAAAADAQABAAABgQC8iBL42yeMBioFay7lty1C4ZDTHcHyo739gc91rTTH8SKvAE4g8Rr97KOz/8PFtOObBrE9G21K7d6UBuPqmd0RUF2CkXXN/eN2PBSHJ50YprRFt/z/304bsBYkDdflKlPDjuSmZ/+OMp4pTsq0R0eNFlX9wcwxEzooIb7VPEdvWE7AYoBRUdf41u3KBHuvjGd1M6QYJtbFLQMMTiVe5IUfyVSZ1RCxEyAB9fR9CBhtVheTVsY3iG0fZc9eCEo89ErDgtGUTJK4Hxt5yCNwI88YaVmkE85cNtw8YwubWQL3/tGZHfbbQ0fynfB4kWNloyRHFr7E1kDxuX5+pbv26EqRdcOVGucNn7hnGU6C1+ejLWdBD7vgsoilFrEaBWF41elJEPKDzpszEijQ9gTrrWeYOQ+x++lvmOdssDu4KvGmj2K/MQTL2jJYrMJ7GDzsUu3XikChRL7zNfS2jYYQLzovboUCgqfPUsVba9hqeX3U67GsJo+hy5MG9RSry4+ucHs=</b>
admin@example:/config/system/…/example@host/> <b>show</b>
algorithm ssh-rsa;
key-data AAAAB3NzaC1yc2EAAAADAQABAAABgQC8iBL42yeMBioFay7lty1C4ZDTHcHyo739gc91rTTH8SKvAE4g8Rr97KOz/8PFtOObBrE9G21K7d6UBuPqmd0RUF2CkXXN/eN2PBSHJ50YprRFt/z/304bsBYkDdflKlPDjuSmZ/+OMp4pTsq0R0eNFlX9wcwxEzooIb7VPEdvWE7AYoBRUdf41u3KBHuvjGd1M6QYJtbFLQMMTiVe5IUfyVSZ1RCxEyAB9fR9CBhtVheTVsY3iG0fZc9eCEo89ErDgtGUTJK4Hxt5yCNwI88YaVmkE85cNtw8YwubWQL3/tGZHfbbQ0fynfB4kWNloyRHFr7E1kDxuX5+pbv26EqRdcOVGucNn7hnGU6C1+ejLWdBD7vgsoilFrEaBWF41elJEPKDzpszEijQ9gTrrWeYOQ+x++lvmOdssDu4KvGmj2K/MQTL2jJYrMJ7GDzsUu3XikChRL7zNfS2jYYQLzovboUCgqfPUsVba9hqeX3U67GsJo+hy5MG9RSry4+ucHs=;
admin@example:/config/system/…/example@host/> <b>leave</b>
</code></pre>

> [!NOTE]
> The `ssh-keygen` program already base64 encodes the public key data,
> so there is no need to use the `text-editor` command, `set` does the
> job.

## Multiple Users

The factory configuration provides three hierarchical user group levels by
default: **guest ⊂ operator ⊂ admin**.  These levels work out-of-the-box
with sensible permissions - operators can configure the system immediately,
while sensitive items (passwords, cryptographic keys) remain protected.

The default levels provide different access to system resources and
configuration:

- **Admin**: Full system access - can manage users, upgrade software,
  restart the system, and modify all configuration including network
  settings, routing, and firewall rules.

- **Operator**: Configuration access - can modify most system settings
  including network interfaces, routing, firewall, hostname, and more.
  *Cannot access* password hashes, cryptographic keys, or perform
  sensitive operations (factory reset, software upgrade).

- **Guest**: Read-only access - can view operational state and
  configuration but cannot modify anything or execute operations.

System access control is handled by the [ietf-netconf-acm][3] YANG model,
usually referred to as [NACM](nacm.md), which provides granular access to
configuration, data, and RPC commands.  The hierarchical levels in the system
are determined by:

1. **NACM permissions** - what the user can access
2. **Shell setting** - which command-line interface the user can use

By default the system ships with a single user, `admin`, in the `admin`
group.  There are no restrictions on the number of users with admin
privileges, nor is the `admin` user reserved or protected -- it can be
removed from the configuration.  However, it is strongly recommended to
keep at least one user with administrator privileges, otherwise the only
way to regain full access is to perform a *factory reset*.

For an overview of users and groups on the system, there is an admin-exec
command:

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
jacky                  clish   password
monitor                false   key

<span class="header">GROUP                  USERS                                    </span>
admin                  admin
operator               jacky
guest                  monitor
</code></pre>

The permissions matrix shows effective access for each NACM group:

- **✓ Full** (green) - unrestricted access
- **⚠ Restricted** (yellow) - access with exceptions, use `show nacm group`
  for details
- **✗ Denied** (red) - no access

For detailed information about a specific group's rules:

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
  comment    : Operators can reboot, shutdown, and set system time.

──────────────────────────────────────────────────────────────────────
<span class="title">deny-password-access (via '*')</span>
  action     : deny
  operations : *
  target     : /ietf-system:system/authentication/user/password
  comment    : No user except admins can access password hashes.

──────────────────────────────────────────────────────────────────────
<span class="title">deny-keystore-access (via '*')</span>
  action     : deny
  operations : *
  target     : ietf-keystore
  comment    : No user except admins can access cryptographic keys.

──────────────────────────────────────────────────────────────────────
<span class="title">deny-truststore-access (via '*')</span>
  action     : deny
  operations : *
  target     : ietf-truststore
  comment    : No user except admins can access trust store.
</code></pre>

For user details:

<pre class="cli"><code>admin@example:/> <b>show nacm user jacky</b>
shell            : clish
login            : password
nacm group       : operator
read permission  : restricted
write permission : restricted
exec permission  : restricted

For detailed rules, use: show nacm group &lt;name&gt;
</code></pre>

### Adding a User

Creating a new user starts with defining the user account in the system:

<pre class="cli"><code>admin@example:/config/> <b>edit system authentication user jacky</b>
admin@example:/config/system/…/user/jacky/> <b>change password</b>
New password:
Retype password:
admin@example:/config/system/…/user/jacky/> <b>leave</b>
</code></pre>

> [!TIP]
> It is also possible to use <kbd>set password ...</kbd> if you have the
> fully crypted and salted string ready.  This can be created offline
> with the [`mkpasswd(1)`][5] tool, or the built-in CLI version <kbd>do
> password encrypt [OPTS]</kbd>.  The `do` prefix is handy for reaching
> all top-level commands when in configure context.

An authorized SSH key can be added the same way as described in the
previous sections.

By default, shell access is disabled (`shell false`).  To allow CLI/SSH
access, set the shell:

<pre class="cli"><code>admin@example:/config/> <b>edit system authentication user jacky</b>
admin@example:/config/system/…/user/jacky/> <b>set shell clish</b>
admin@example:/config/system/…/user/jacky/> <b>leave</b>
</code></pre>

Available shells:

- `bash` - Full Bourne-again shell (recommended for admins only)
- `sh` - POSIX shell (recommended for admins only)
- `clish` - Limited CLI-only shell (recommended for operators and guests)
- `false` - No shell access (default)

> [!WARNING] Security Notice
> For security reasons, it is strongly recommended to limit non-admin users
> to the `clish` shell, which provides CLI access without exposing the
> underlying UNIX system.  Reserve `bash` and `sh` for administrators who
> need full system access for debugging and maintenance.
>
> Note that shell and CLI access is not always necessary - the system
> supports NETCONF and RESTCONF for remote management and automation.
> Setting `shell false` for users who only need programmatic access
> minimizes the attack surface and improves overall system security.

### Adding a User to a Group

To assign a user to a specific privilege level, add them to the
corresponding NACM group:

**Operator user:**

<pre class="cli"><code>admin@example:/config/> <b>edit nacm group operator</b>
admin@example:/config/nacm/group/operator/> <b>set user-name jacky</b>
admin@example:/config/nacm/group/operator/> <b>leave</b>
</code></pre>

**Adding another admin:**

<pre class="cli"><code>admin@example:/config/> <b>edit nacm group admin</b>
admin@example:/config/nacm/group/admin/> <b>set user-name alice</b>
admin@example:/config/nacm/group/admin/> <b>leave</b>
</code></pre>

**Guest user:**

<pre class="cli"><code>admin@example:/config/> <b>edit nacm group guest</b>
admin@example:/config/nacm/group/guest/> <b>set user-name monitor</b>
admin@example:/config/nacm/group/guest/> <b>leave</b>
</code></pre>

> [!TIP]
> For technical details about NACM rule evaluation, module-name vs path
> matching, and creating custom access control policies, see the
> [NACM Technical Guide](nacm.md).

### Access Control Matrix

The following table shows what each user level can do based on the NACM rules
and shell access configured for each user:

- **Admin**: `bash` — full system access
- **Operator**: `clish` — CLI-only access without UNIX system exposure
- **Guest**: `false` — no shell access

| Feature                | Admin | Operator | Guest     |
|------------------------|-------|----------|-----------|
| Network interfaces     | ✓     | ✓        | Read only |
| Routing (FRR)          | ✓     | ✓        | Read only |
| Firewall rules         | ✓     | ✓        | Read only |
| VLANs/bridges          | ✓     | ✓        | Read only |
| Containers             | ✓     | ✓        | Read only |
| Hostname/system config | ✓     | ✓        | Read only |
| CLI/SSH access         | ✓     | ✓        | ✗         |
| System restart         | ✓     | ✓        | ✗         |
| Set date/time          | ✓     | ✓        | ✗         |
| System reboot          | ✓     | ✓        | ✗         |
| System shutdown        | ✓     | ✓        | ✗         |
| User management        | ✓     | ✗        | Read only |
| Keystore (certs/keys)  | ✓     | ✗        | ✗         |
| Truststore             | ✓     | ✗        | ✗         |
| Read passwords/secrets | ✓     | ✗        | ✗         |
| NACM rules             | ✓     | ✗        | ✗         |
| Factory reset          | ✓     | ✗        | ✗         |
| Software upgrade       | ✓     | ✗        | ✗         |

### Security Aspects

The three default user levels are implemented through a combination of NACM
rules and UNIX group membership.  Access control is permission-based, not
name-based - the system detects user levels by examining their NACM
permissions and shell settings.

**Admin users** have unrestricted NACM access with the following rule:

```json
   "module-name": "*",
   "access-operations": "*",
   "action": "permit"
```

Admin users are automatically added to the UNIX `wheel` and `frrvty`
groups, granting them `sudo` privileges and access to FRR routing
protocols. This makes it possible to use all the underlying UNIX
tooling, which can be very useful for debugging, but please use with
care -- the system is designed to be managed through the CLI and
NETCONF, not directly via shell commands.

**Operator users** use the permit-by-default NACM model (`write-default:
"permit"`), which means they can configure most system settings without
explicit permit rules. This design is "future proof" - when new features
are added, operators can immediately use them.

The following are explicitly denied to operators through global NACM rules:

- Password hashes (`/ietf-system:system/authentication/user/password`)
- Cryptographic keys (`ietf-keystore` module)
- Trust store certificates (`ietf-truststore` module)

Additionally, sensitive operations like factory reset, software upgrades,
and system shutdown are protected by YANG-level `nacm:default-deny-all`
annotations and remain restricted to administrators.

Operators are automatically added to the UNIX `operator` and `frrvty`
groups, granting them `sudo` privileges for network operations and FRR
access.

**Guest users** have read-only NACM access through an explicit deny rule
that blocks all write and exec operations (`create update delete exec`),
while `read-default: "permit"` allows viewing configuration and state.
Guests receive no special UNIX group memberships. The shell setting
determines whether guests can access the CLI (`clish`) or are restricted
from shell access entirely (`false`).

All users, regardless of level, are denied access to password hashes and
cryptographic key material through global NACM rules.

## Changing Hostname

Notice how the hostname in the prompt does not change until the change
is committed by issuing the `leave` command.

<pre class="cli"><code>admin@example:/config/> <b>edit system</b>
admin@example:/config/system/> <b>set hostname myrouter</b>
admin@example:/config/system/> <b>leave</b>
admin@myrouter:/>
</code></pre>

The hostname is advertised over mDNS-SD in the `.local` domain.  If
another device already has claimed the `myrouter.local` CNAME, in our
case, mDNS will advertise a "uniqified" variant, usually suffixing with
an index, e.g., `myrouter-1.local`.  Use an mDNS browser to scan for
available devices on your LAN.

In some cases you may want to set the device's *domain name* as well.
This is handled the same way:

<pre class="cli"><code>admin@example:/config/> <b>edit system</b>
admin@example:/config/system/> <b>set hostname foo.example.com</b>
admin@example:/config/system/> <b>leave</b>
admin@foo:/>
</code></pre>

Both host and domain name are stored in the system files `/etc/hosts`
and `/etc/hostname`.  The latter is exclusively for the host name.  The
domain *may* be used by the system DHCP server when handing out leases
to clients, it is up to the clients to request the domain name *option*.

> [!NOTE]
> Critical services like syslog, mDNS, LLDP, and similar that advertise
> the hostname, are restarted when the hostname is changed.


## Changing Login Banner

The `motd-banner` setting is an Infix augment and an example of a
`binary` type setting that can be changed interactively with the
built-in [`text-editor` command](cli/text-editor.md).

> [!TIP]
> See the next section for how to change the editor used to something
> you may be more familiar with.

<pre class="cli"><code>admin@example:/config/> <b>edit system</b>
admin@example:/config/system/> <b>text-editor motd-banner</b>
admin@example:/config/system/> <b>leave</b>
admin@example:/>
</code></pre>

Log out and log back in again to inspect the changes.


## Changing the Editor

The system has three different built-in editors that can be used
as the `text-editor` command:

 - `emacs` (Micro Emacs)
 - `nano` (GNU Nano)
 - `vi` (Visual Editor)

To change the editor to GNU Nano:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit system</b>
admin@example:/config/system/> <b>set text-editor nano</b>
admin@example:/config/system/> <b>leave</b>
admin@example:/>
</code></pre>

> [!IMPORTANT]
> Configuration changes only take effect after issuing the `leave`
> command.  I.e., you must change the editor first, and then re-enter
> configure context to use your editor of choice.


## DNS Resolver Configuration

The system supports both static and dynamic (DHCP) DNS setup.  The
locally configured (static) server is preferred over any acquired
from a DHCP client.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit system dns-resolver</b>
admin@example:/config/system/dns-resolver/> <b>set server google udp-and-tcp address 8.8.8.8</b>
admin@example:/config/system/dns-resolver/> <b>show</b>
server google {
  udp-and-tcp {
    address 8.8.8.8;
  }
}
admin@example:/config/system/dns-resolver/> <b>leave</b>
</code></pre>

It is also possible to configure resolver options like timeout and
retry attempts.  See the YANG model for details, or use the built-in
help system in the CLI.

> [!NOTE]
> When acting as a DHCP server and DNS proxy for other devices, any
> local DNS server configured here is automatically used as upstream DNS
> server.


## NTP Client Configuration

Below is an example configuration for enabling NTP with a specific
server and the `iburst` option for faster initial synchronization.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit system ntp</b>
admin@example:/config/system/ntp/> <b>set enabled</b>
admin@example:/config/system/ntp/> <b>set server ntp-pool</b>
admin@example:/config/system/ntp/> <b>set server ntp-pool udp address pool.ntp.org</b>
admin@example:/config/system/ntp/> <b>set server ntp-pool iburst</b>
admin@example:/config/system/ntp/> <b>set server ntp-pool prefer</b>
admin@example:/config/system/ntp/> <b>leave</b>
</code></pre>

This configuration enables the NTP client and sets the NTP server to
`pool.ntp.org` with the `iburst` and `prefer` options. The `iburst`
option ensures faster initial synchronization, and the `prefer` option
designates this server as preferred.

* `prefer false`: The NTP client will choose the best available source
based on several factors, such as network delay, stratum, and other
metrics (default config).
* `prefer true`: The NTP client will try to use the preferred server
as the primary source unless it becomes unreachable or unusable.


### Show NTP Sources

The status for NTP sources is availble in YANG and accessable with
CLI/NETCONF/RESTCONF.

To view the sources being used by the NTP client, run:

<pre class="cli"><code>admin@example:/> <b>show ntp</b>
Mode                : Client
Stratum             : 3
Ref time (UTC)      : Sat Jan 24 23:41:42 2026

<span class="header">ADDRESS         MODE    STATE     STRATUM  POLL</span>
147.78.228.41   server  outlier         2   64s
192.168.0.1     server  unusable        0  128s
176.126.86.247  server  selected        2   64s
</code></pre>

### Show NTP Status

To check the status of NTP synchronization (only availble in CLI), use
the following command:

<pre class="cli"><code>admin@example:/> <b>show ntp tracking</b>
Reference ID        : 176.126.86.247
Stratum             : 3
Ref time (UTC)      : Sat Jan 24 23:41:42 2026
System time         : 0.000000 seconds slow of NTP time
Last offset         : -454779.375000000 seconds
RMS offset          : 454779.375000000 seconds
Frequency           : 0.000 ppm slow
Residual freq       : -26.383 ppm
Skew                : 1000000.000 ppm
Root delay          : 0.007395 seconds
Root dispersion     : 39.181149 seconds
Update interval     : 0.0 seconds
Leap status         : Normal
</code></pre>

This output provides detailed information about the NTP status, including
reference ID, stratum, time offsets, frequency, and root delay.

> [!TIP]
> The system uses `chronyd` Network Time Protocol (NTP) daemon.  The
> output shown here is best explained in the [Chrony documentation][4].

[1]: https://www.rfc-editor.org/rfc/rfc7317
[2]: https://github.com/kernelkit/infix/blob/main/src/confd/yang/infix-system%402024-02-29.yang
[3]: https://www.rfc-editor.org/rfc/rfc8341
[4]: https://chrony-project.org/doc/4.6.1/chronyc.html
[5]: https://linux.die.net/man/1/mkpasswd
