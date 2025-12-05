# System Configuration

System settings in Infix are provided by the [ietf-system][1] YANG
model, augmented with Linux specific extensions in [infix-system][2],
like Message of the Day (login message) and user login shell.  More
on this later on in this document.

For the sake of brevity, the hostname in the following examples has been
shortened to `host`.  The default hostname is composed from a product
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

```
admin@host:/config/> edit system authentication user admin
admin@host:/config/system/authentication/user/admin/> change password
New password:
Retype password:
admin@host:/config/system/authentication/user/admin/> leave
```

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

```
admin@host:/config/> edit system authentication user admin
admin@host:/config/system/authentication/user/admin/> edit authorized-key example@host
admin@host:/config/system/authentication/user/admin/authorized-key/example@host/> set algorithm ssh-rsa
admin@host:/config/system/authentication/user/admin/authorized-key/example@host/> set key-data AAAAB3NzaC1yc2EAAAADAQABAAABgQC8iBL42yeMBioFay7lty1C4ZDTHcHyo739gc91rTTH8SKvAE4g8Rr97KOz/8PFtOObBrE9G21K7d6UBuPqmd0RUF2CkXXN/eN2PBSHJ50YprRFt/z/304bsBYkDdflKlPDjuSmZ/+OMp4pTsq0R0eNFlX9wcwxEzooIb7VPEdvWE7AYoBRUdf41u3KBHuvjGd1M6QYJtbFLQMMTiVe5IUfyVSZ1RCxEyAB9fR9CBhtVheTVsY3iG0fZc9eCEo89ErDgtGUTJK4Hxt5yCNwI88YaVmkE85cNtw8YwubWQL3/tGZHfbbQ0fynfB4kWNloyRHFr7E1kDxuX5+pbv26EqRdcOVGucNn7hnGU6C1+ejLWdBD7vgsoilFrEaBWF41elJEPKDzpszEijQ9gTrrWeYOQ+x++lvmOdssDu4KvGmj2K/MQTL2jJYrMJ7GDzsUu3XikChRL7zNfS2jYYQLzovboUCgqfPUsVba9hqeX3U67GsJo+hy5MG9RSry4+ucHs=
admin@host:/config/system/authentication/user/admin/authorized-key/example@host/> show
algorithm ssh-rsa;
key-data AAAAB3NzaC1yc2EAAAADAQABAAABgQC8iBL42yeMBioFay7lty1C4ZDTHcHyo739gc91rTTH8SKvAE4g8Rr97KOz/8PFtOObBrE9G21K7d6UBuPqmd0RUF2CkXXN/eN2PBSHJ50YprRFt/z/304bsBYkDdflKlPDjuSmZ/+OMp4pTsq0R0eNFlX9wcwxEzooIb7VPEdvWE7AYoBRUdf41u3KBHuvjGd1M6QYJtbFLQMMTiVe5IUfyVSZ1RCxEyAB9fR9CBhtVheTVsY3iG0fZc9eCEo89ErDgtGUTJK4Hxt5yCNwI88YaVmkE85cNtw8YwubWQL3/tGZHfbbQ0fynfB4kWNloyRHFr7E1kDxuX5+pbv26EqRdcOVGucNn7hnGU6C1+ejLWdBD7vgsoilFrEaBWF41elJEPKDzpszEijQ9gTrrWeYOQ+x++lvmOdssDu4KvGmj2K/MQTL2jJYrMJ7GDzsUu3XikChRL7zNfS2jYYQLzovboUCgqfPUsVba9hqeX3U67GsJo+hy5MG9RSry4+ucHs=;
admin@host:/config/system/authentication/user/admin/authorized-key/example@host/> leave
```

> [!NOTE]
> The `ssh-keygen` program already base64 encodes the public key data,
> so there is no need to use the `text-editor` command, `set` does the
> job.


## Multiple Users

The system supports multiple users and multiple user levels, or groups,
that a user can be a member of.  Access control is entirely handled by
the NETCONF ["NACM"][3] YANG model, which provides granular access to
configuration, data, and RPC commands over NETCONF.

By default the system ships with a single group, `admin`, which the
default user `admin` is a member of.  The broad permissions granted by
the `admin` group is what gives its users full system administrator
privileges.  There are no restrictions on the number of users with
administrator privileges, nor is the `admin` user reserved or protected
in any way -- it is completely possible to remove the default `admin`
user from the configuration.  However, it is recommended to keep at
least one user with administrator privileges in the system, otherwise
the only way to regain full access is to perform a *factory reset*.

### Adding a User

Similar to how to change password, adding a new user is done using the
same set of commands:

```
admin@host:/config/> edit system authentication user jacky
admin@host:/config/system/authentication/user/jacky/> change password
New password:
Retype password:
admin@host:/config/system/authentication/user/jacky/> leave
```

An authorized SSH key is added the same way as presented previously.

### Adding a User to the Admin Group

The following commands add user `jacky` to the `admin` group.

```
admin@host:/config/> edit nacm group admin
admin@host:/config/nacm/group/admin/> set user-name jacky
admin@host:/config/nacm/group/admin/> leave
```

### Security Aspects

The NACM user levels apply primarily to NETCONF, with exception of the
`admin` group which is granted full system administrator privileges to
the underlying UNIX system with the following ACL rules:

```json
   ...
   "module-name": "*",
   "access-operations": "*",
   "action": "permit",
   ...
```

A user in the `admin` group is allowed to also use a POSIX login shell
and use the `sudo` command to perform system administrative commands.
This makes it possible to use all the underlying UNIX tooling, which
to many can be very useful, in particular when debugging a system, but
please remember to use with care -- the system is not built to require
managing from the shell.  The tools available in the CLI and automated
services, started from the system's configuration, are the recommended
way of using the system, in addition to NETCONF tooling.


## Changing Hostname

Notice how the hostname in the prompt does not change until the change
is committed by issuing the `leave` command.

```
admin@host:/config/> edit system
admin@host:/config/system/> set hostname example
admin@host:/config/system/> leave
admin@example:/>
```

The hostname is advertised over mDNS-SD in the `.local` domain.  If
another device already has claimed the `example.local` CNAME, in our
case, mDNS will advertise a "uniqified" variant, usually suffixing with
an index, e.g., `example-1.local`.  Use an mDNS browser to scan for
available devices on your LAN.

In some cases you may want to set the device's *domain name* as well.
This is handled the same way:

```
admin@host:/config/> edit system
admin@host:/config/system/> set hostname foo.example.com
admin@host:/config/system/> leave
admin@foo:/>
```

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

```
admin@host:/config/> edit system
admin@host:/config/system/> text-editor motd-banner
admin@host:/config/system/> leave
admin@host:/>
```

Log out and log back in again to inspect the changes.


## Changing the Editor

The system has three different built-in editors that can be used
as the `text-editor` command:

 - `emacs` (Micro Emacs)
 - `nano` (GNU Nano)
 - `vi` (Visual Editor)

To change the editor to GNU Nano:

```
admin@host:/> configure
admin@host:/config/> edit system
admin@host:/config/system/> set text-editor nano
admin@host:/config/system/> leave
admin@host:/>
```

> [!IMPORTANT]
> Configuration changes only take effect after issuing the `leave`
> command.  I.e., you must change the editor first, and then re-enter
> configure context to use your editor of choice.


## DNS Resolver Configuration

The system supports both static and dynamic (DHCP) DNS setup.  The
locally configured (static) server is preferred over any acquired
from a DHCP client.

```
admin@host:/> configure
admin@host:/config/> edit system dns-resolver
admin@host:/config/system/dns-resolver/> set server google udp-and-tcp address 8.8.8.8
admin@host:/config/system/dns-resolver/> show
server google {
  udp-and-tcp {
    address 8.8.8.8;
  }
}
admin@host:/config/system/dns-resolver/> leave
```

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

```
admin@host:/> configure
admin@host:/config/> edit system ntp
admin@host:/config/system/ntp/> set enabled
admin@host:/config/system/ntp/> set server ntp-pool
admin@host:/config/system/ntp/> set server ntp-pool udp address pool.ntp.org
admin@host:/config/system/ntp/> set server ntp-pool iburst
admin@host:/config/system/ntp/> set server ntp-pool prefer
admin@host:/config/system/ntp/> leave
```

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
```
admin@target:/> show ntp
ADDRESS         MODE         STATE            STRATUM POLL-INTERVAL
192.168.1.1     server       candidate              1             6
192.168.2.1     server       candidate              1             6
192.168.3.1     server       selected               1             6
```

### Show NTP Status

To check the status of NTP synchronization (only availble in CLI), use
the following command:

```
admin@host:/> show ntp tracking
Reference ID    : C0248F86 (192.36.143.134)
Stratum         : 2
Ref time (UTC)  : Mon Oct 21 10:06:45 2024
System time     : 0.000000001 seconds slow of NTP time
Last offset     : -3845.151367188 seconds
RMS offset      : 3845.151367188 seconds
Frequency       : 4.599 ppm slow
Residual freq   : +1293.526 ppm
Skew            : 12.403 ppm
Root delay      : 1.024467230 seconds
Root dispersion : 0.273462683 seconds
Update interval : 0.0 seconds
Leap status     : Normal
admin@host:/>
```

This output provides detailed information about the NTP status, including
reference ID, stratum, time offsets, frequency, and root delay.

> [!TIP]
> The system uses `chronyd` Network Time Protocol (NTP) daemon.  The
> output shown here is best explained in the [Chrony documentation][4].

[1]: https://www.rfc-editor.org/rfc/rfc7317
[2]: https://github.com/kernelkit/infix/blob/main/src/confd/yang/infix-system%402024-02-29.yang
[3]: https://www.rfc-editor.org/rfc/rfc8341
[4]: https://chrony-project.org/doc/4.6.1/chronyc.html
