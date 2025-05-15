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

## Upgrade procedures and boot order

For resilience purposes, Infix maintains two software
images referred to as the _primary_ and _secondary_ partition image.
In addition, some bootloaders support [netbooting][6].

The _boot order_ defines which image is tried first, and is listed
with the CLI `show software` command. It also shows Infix version
installed per partition, and which image was used when booting (`STATE
booted`).

```
admin@example:/> show software
BOOT ORDER
primary secondary net

NAME      STATE     VERSION                DATE
primary   booted    v25.01.0               2025-04-25T10:15:00+00:00
secondary inactive  v25.01.0               2025-04-25T10:07:20+00:00
admin@example:/>
```

YANG support for upgrading Infix, inspecting and _modifying_ the
boot-order, is defined in [infix-system-software][5].


### Upgrading Infix

Upgrading Infix is done one partition at a time. If the system has
booted from one partition, an `upgrade` will apply to the other
(inactive) partition.

1. Download and unpack the release to install. Make the image *pkg*
   bundle available at some URL[^10]
2. (Optional) Backup the startup configuration
3. Assume the unit has booted the `primary` image. Then running the
   `upgrade` command installs a new image on the `secondary`
   partition
4. As part of a successful upgrade, the boot-order is implictly
   changed to boot the newly installed image
5. Reboot the unit
6. The unit now runs the new image. To upgrade the remaining partition
   (`primary`), run the same upgrade command again, and (optionally)
   reboot to verify the upgrade
   
> [!CAUTION]
> During boot (step 5), the unit may
> [migrate](#configuration-migration) the startup configuration for
> any syntax changes.  It is therefore important that you make sure to
> upgrade the other partition as well after reboot, of course after
> having verified your setup.

The CLI example below shows steps 2-5.

*Backup startup configuration:* It is recommended to backup the
startup configuration before performing an upgrade. The backup is
useful if the upgrade fails, and makes a later
[downgrade](#downgrading-infix) smoother to conduct.

```
admin@example:/> dir /cfg
/cfg directory
backup/             ssl/                startup-config.cfg

admin@example:/> copy /cfg/startup-config.cfg /cfg/v25.01.0-startup-config.cfg
admin@example:/> dir /cfg
/cfg directory
backup/             ssl/                startup-config.cfg           v25.01.0-startup-config.cfg

admin@example:/> 
```

*Upgrade:* Here the image *pkg bundle* was made available via TFTP.

```
admin@example:/> upgrade tftp://198.18.117.1/infix-aarch64-25.03.1.pkg
installing
  0% Installing
  0% Determining slot states
 10% Determining slot states done.
...
 98% Copying image to rootfs.1
 99% Copying image to rootfs.1
 99% Copying image to rootfs.1 done.
 99% Updating slots done.
100% Installing done.
Installing `tftp://198.18.117.1/infix-aarch64-25.03.1.pkg` succeeded
admin@example:/>
```

*Reboot:* The unit will boot on the other partition, with the newly
installed image. The `Loading startup-config` step conducts migration
of startup configuration if applicable.

```
admin@example:/> reboot
[ OK ] Stopping Static routing daemon
[ OK ] Stopping Zebra routing daemon
...
[ OK ] Loading startup-config
[ OK ] Verifying self-signed https certificate
[ OK ] Update DNS configuration
[ OK ] Starting Status daemon

Infix -- a Network Operating System v25.03.1 (ttyS0)
example login: admin
Password:
.-------.
|  . .  | Infix -- a Network Operating System
|-. v .-| https://kernelkit.org
'-'---'-'

Run the command 'cli' for interactive OAM

admin@example:~$ cli

See the 'help' command for an introduction to the system

admin@example:/> show software
BOOT ORDER
secondary primary net

NAME      STATE     VERSION                DATE
primary   inactive  v25.01.0               2025-04-25T10:15:00+00:00
secondary booted    v25.03.1               2025-04-25T10:24:31+00:00
admin@example:/>
```

As shown, the *boot order* has been updated, so that *secondary* is
now the preferred boot source.

To upgrade the remaining partition (`primary`), run the `upgrade URL`
command again, and (optionally) reboot.

### Configuration Migration

The example above illustrated an upgrade from Infix v25.01.0 to
v25.03.1. Inbetween these versions, YANG configuration definitions
changed slightly (more details given below).

During boot, Infix inspects the `version` meta information within the
startup configuration file to determine if configuration migration is
needed. In this specific case, the configuration file has version
`1.4` while the booted software expects version `1.5` (the
configuration version numbering differs from the Infix image version
numbering).  The startup configuration is migrated to `1.5`
definitions and stored, while a backup previous startup configuration
is stored in directory `/cfg/backup/`.

```
admin@example:/> dir /cfg/backup/
/cfg/backup/ directory
startup-config-1.4.cfg

admin@example:/>
```

The modifications made to the startup configuration can be viewed by
comparing the files from the *shell*. An example is shown below.

```
admin@example:/> exit
admin@example:~$ diff /cfg/backup/startup-config-1.4.cfg /cfg/startup-config.cfg
--- /cfg/backup/startup-config-1.4.cfg
+++ /cfg/startup-config.cfg
...
-          "public-key-format": "ietf-crypto-types:ssh-public-key-format",
+          "public-key-format": "infix-crypto-types:ssh-public-key-format",
...
-          "private-key-format": "ietf-crypto-types:rsa-private-key-format",
+          "private-key-format": "infix-crypto-types:rsa-private-key-format",
...
-    "version": "1.4"
+    "version": "1.5"
...
admin@example:~$
```

### Downgrading Infix

Downgrading to an earlier Infix version is possible, however,
downgrading is **not** guaranteed to work smoothly. In particular,
when the unit boots up with the downgraded version, it may fail to
apply the *startup config*, and instead apply its [failure config][7].

We consider two cases: downgrading with or without applying a backup
startup configuration before rebooting.

In both cases we start out with a unit running Infix v25.03.1, and
wish to downgrade to v25.01.0.

```
admin@example:/> show software
BOOT ORDER
primary secondary net

NAME      STATE     VERSION                DATE
primary   booted    v25.03.1               2025-04-25T11:36:26+00:00
secondary inactive  v25.03.1               2025-04-25T10:24:31+00:00
admin@example:/>
```

#### Downgrading when applying a backup startup configuration

This is the recommended approach to downgrade, given that you have a
backup configuration available.  The objective is to avoid ending up
with the unit in *failure config*.

1. Find the backup configuration file.
2. Run `upgrade URL` to install Infix image to downgrade to.
3. Copy backup startup configuration to current startup configuration
   (from shell).
4. Reboot.

*Find the backup configuration file:*

Assume you have a backup startup config for the Infix version to
downgrade to (here Infix v25.01.0, config `version 1.4`).

The preferred approach is to use a startup configuration backed up
when running Infix v25.01.0 on the unit. See the section on [upgrading
Infix](#upgrading-infix) above for more information. In the example
below, there is a backup file available named
*v25.01.0-startup-config.cfg* 

```
admin@example:/> dir /cfg
/cfg directory
backup/             ssl/                startup-config.cfg           v25.01.0-startup-config.cfg

admin@example:/> 
```

The alternative is to use a startup config implicitly backed up by the
system as part of [configuration migration](#configuration-migration).

```
admin@example:/> dir /cfg/backup/
/cfg/backup/ directory
startup-config-1.4.cfg

admin@example:/>
```

> [!CAUTION] Using a backup configuration file stored when the unit
> was running the old version (e.g., v25.01.0-startup-config.cfg) is
> preferred. Although backup files stored due to configuration
> migration (e.g., startup-config-1.4.cfg) usually works too if the
> configuration file version (`1.4`) matches, there are
> situations when the system may fail to apply it as described below.

The *configuration file version* (`1.4`) is only incremented when
changes in YANG configuration syntax mandates it to handle
*upgrading*.  Say the next Infix version includes a new feature
setting, it can still have version `1.4`, as upgrading to it would not
need migration. If a user then enables the new feature setting, the
new configuration will no longer be compatible with the previous *Infix
version*. A downgrade after enabling new features risks ending up with
the unit in *failure config*. 


*Use `upgrade` command to downgrade:*

```
admin@example:/> upgrade tftp://198.18.117.1/infix-aarch64-25.01.0.pkg
installing
  0% Installing
  0% Determining slot states
 10% Determining slot states done.
 ...
 99% Copying image to rootfs.1 done.
 99% Updating slots done.
100% Installing done.
Installing `tftp://198.18.117.1/infix-aarch64-25.01.0.pkg` succeeded
admin@example:/>
```

*Apply the backup configuration file:*

It is recommended to use a backup configuration file for the Infix version to
downgrade to, if there is one available.

```
admin@example:/> copy /cfg/v25.01.0-startup-config.cfg /cfg/startup-config.cfg
Overwrite existing file /cfg/startup-config.cfg (y/N)? y
admin@example:/>
```

An alternative is to use a backup file stored when the system
conducted a [configuration migration](#configuration-migration). See
the *caution* note above.

```
admin@example:/> copy /cfg/backup/startup-config-1.4.cfg /cfg/startup-config.cfg
Overwrite existing file /cfg/startup-config.cfg (y/N)? y
admin@example:/>
```

*Reboot:*

The unit will come up with the applied backup configuration. 

```
admin@example:/> reboot
[ OK ] Saving system clock to file
[ OK ] Stopping Software update service
[ OK ] Stopping Status daemon
...
[ OK ] Bootstrapping YANG datastore
[ OK ] Starting Configuration daemon
[ OK ] Loading startup-config
[ OK ] Update DNS configuration
[ OK ] Verifying self-signed https certificate
[ OK ] Starting Status daemon

Infix -- a Network Operating System v25.01.0 (ttyS0)
example login:
```
> [!NOTE]
> If the unit despite these measures ends up in *failure config*, see
> the next section for more information on how to recover.

#### Downgrading without applying a backup startup configuration

This procedure assumes you have access to the unit's console port and
its default login credentials[^9].

1. Downgrade
2. Reboot
3. Login with unit's default credentials
4. Conduct factory reset
5. (Then go on configure the unit as you wish)

*Use `upgrade` command to downgrade:*

```
admin@example:/> upgrade tftp://198.18.117.1/infix-aarch64-25.01.0.pkg
installing
  0% Installing
  0% Determining slot states
 10% Determining slot states done.
 ...
 99% Copying image to rootfs.1 done.
 99% Updating slots done.
100% Installing done.
Installing `tftp://198.18.117.1/infix-aarch64-25.01.0.pkg` succeeded
admin@example:/>
```

*Reboot:*

Conduct a reboot. During boot, the unit fails to apply the existing
startup configuration (config version `1.5` while software expects
version `1.4` or earlier), and instead applies its [failure
config][7]. This is what is seen on the console when this situation
occurs. Note that the login prompt displays `failed` as part of the
*hostname*.

```
admin@example:/> reboot
[ OK ] Saving system clock to file
[ OK ] Stopping Software update service
[ OK ] Stopping Status daemon
...
[ OK ] Verifying SSH host keys
[ OK ] Bootstrapping YANG datastore
[ OK ] Starting Configuration daemon
[FAIL] Loading startup-config
[ OK ] Loading failure-config
[ OK ] Verifying self-signed https certificate
[ OK ] Starting Status daemon

Infix -- a Network Operating System v25.01.0 (ttyS0)

ERROR: Corrupt startup-config, system has reverted to default login credentials
failed-00-00-00 login:
```

To remedy a situation like this, you can login with the unit's *default
login credentials*, preferrably via a [console port][8].
The unit's default credentials are typically printed on a sticker on
the unit.

```
failed-00-00-00 login: admin
Password:

Run the command 'cli' for interactive OAM

admin@failed-00-00-00:~$
```

When it is *safe* from a network operations perspective, you can
conduct a factory reset and reboot. It is recommended to remove the
unit from any production network before doing this, as a factory reset
may enable undesired connectivity between the unit's ports.

```
admin@failed-00-00-00:~$ factory
Factory reset device (y/N)? y
factory: scheduled factory reset on next boot.
Reboot now to perform reset, (y/N)? y
[ OK ] Saving system time (UTC) to RTC
[ OK ] Stopping mDNS alias advertiser
...
[ OK ] Starting Configuration daemon
[ OK ] Loading startup-config
[ OK ] Update DNS configuration
[ OK ] Verifying self-signed https certificate
[ OK ] Starting Status daemon
[ OK ] Starting Status daemon


Please press Enter to activate this console.

Infix -- a Network Operating System v25.01.0 (ttyS0)
example login:
```

Continued configuration is done as with any unit after factory reset.

[1]: https://www.rfc-editor.org/rfc/rfc7317
[2]: https://github.com/kernelkit/infix/blob/main/src/confd/yang/infix-system%402024-02-29.yang
[3]: https://www.rfc-editor.org/rfc/rfc8341
[4]: https://chrony-project.org/doc/4.6.1/chronyc.html
[5]: https://github.com/kernelkit/infix/blob/main/src/confd/yang/confd/infix-system-software.yang
[6]: boot.md#system-configuration
[7]: introduction.md#system-boot
[8]: management.md#console-port
[^9]: In failure config, Infix puts all Ethernet ports as individual
    interfaces. With direct access, one can connect with e.g., SSH,
    using link local IPv6 addresses. This as an alternative to
    connecting via a console port.
[^10]: Set up an FTP/TFTP/SFTP or HTTP/HTTPS server on the same LAN. 

[11]: scripting.md#-backup-configuration-using-sysrepocfg-and-scp
