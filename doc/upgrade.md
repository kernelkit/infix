# Upgrading & Boot Order

For resilience purposes, Infix maintains two software images referred to
as the _primary_ and _secondary_ partition image.  In addition, some
bootloaders support [netbooting][1].

The _boot order_ defines which image is tried first, and is listed with
the CLI `show software` command. It also shows Infix version installed
per partition, and which image was used when booting (`STATE booted`).

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
boot-order, is defined in [infix-system-software][2].


## Upgrading

Upgrading Infix is done one partition at a time. If the system has
booted from one partition, an `upgrade` will apply to the other
(inactive) partition.

1. Download and unpack the release to install. Make the image *pkg*
   bundle available at some URL[^2]
2. (Optional) Backup the startup configuration
3. Assume the unit has booted the `primary` image. Then running the
   `upgrade` command installs a new image on the `secondary` partition
4. As part of a successful upgrade, the boot-order is implictly
   changed to boot the newly installed image
5. Reboot the unit
6. The unit now runs the new image. To upgrade the remaining partition
   (`primary`), run the same upgrade command again, and (optionally)
   reboot to verify the upgrade

> [!CAUTION]
> During boot (step 5), the unit may [migrate](#configuration-migration)
> the startup configuration for any syntax changes.  It is therefore
> important that you make sure to upgrade the other partition as well
> after reboot, of course after having verified your setup.

The CLI example below shows steps 2-5.

*Backup startup configuration:* It is recommended to backup the startup
configuration before performing an upgrade. The backup is useful if the
upgrade fails, and makes a later [downgrade](#downgrading) a smoother
process.

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

Infix OS — Immutable.Friendly.Secure v25.03.1 (ttyS0)
example login: admin
Password:
.-------.
|  . .  | Infix OS — Immutable.Friendly.Secure
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

## Configuration Migration

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

## Downgrading

Downgrading to an earlier version is possible, however, downgrading is
**not** guaranteed to work smoothly.  In particular, when the unit boots
up with the downgraded version, it may fail to apply the *startup
config*, and instead apply its [failure config][3].

We consider two cases: downgrading with and without applying a backup
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

### With Backup `startup-config`

This is the recommended approach to downgrade, given that you have a
backup configuration available.  The objective is to avoid ending up
with the unit in *failure config*.

1. Find the backup configuration file
1. Run `upgrade URL` to install Infix image to downgrade to
1. Copy backup startup configuration to current startup configuration
   (from shell)
1. Reboot

*Find the backup configuration file:*

Assume you have a backup startup config for the Infix version to
downgrade to (here Infix v25.01.0, config `version 1.4`).

The preferred approach is to use a startup configuration backed up when
running Infix v25.01.0 on the unit.  See section [Upgrading](#upgrading)
above for more information.  In the following example, there is a backup
file available named `v25.01.0-startup-config.cfg`:

```
admin@example:/> dir /cfg
/cfg directory
backup/       ssl/       startup-config.cfg    v25.01.0-startup-config.cfg

admin@example:/> 
```

The alternative is to use a startup config implicitly backed up by the
system as part of [Configuration Migration](#configuration-migration).

```
admin@example:/> dir /cfg/backup/
/cfg/backup/ directory
startup-config-1.4.cfg

admin@example:/>
```

> [!CAUTION]
> Using a backup configuration file stored when the unit was running the
> old version (e.g., v25.01.0-startup-config.cfg) is preferred.  Although
> backup files stored due to configuration migration (e.g.,
> `startup-config-1.4.cfg`) usually works too if the configuration file
> version (`1.4`) matches, there are situations when the system may fail
> to apply it as described below.

The *configuration file version* (`1.4`) is only incremented when
changes in YANG configuration syntax mandates it to handle *upgrading*.
Say the next Infix version includes a new feature setting, it can still
have version `1.4`, as upgrading to it would not need migration. If a
user then enables the new feature setting, the new configuration will no
longer be compatible with the previous *Infix version*. A downgrade
after enabling new features risks ending up with the unit in *failure
config*.

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

Infix OS — Immutable.Friendly.Secure v25.01.0 (ttyS0)
example login:
```

> [!NOTE]
> If the unit despite these measures ends up in *failure config*, see
> the next section for more information on how to recover.

### Without a Backup `startup-config`

This procedure assumes you have access to the unit's console port and
its default login credentials[^1].

1. Downgrade
1. Reboot
1. Login with unit's default credentials
1. Conduct factory reset
1. (Then go on configure the unit as you wish)

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
config][3]. This is what is seen on the console when this situation
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

Infix OS — Immutable.Friendly.Secure v25.01.0 (ttyS0)

ERROR: Corrupt startup-config, system has reverted to default login credentials
failed-00-00-00 login:
```

To remedy a situation like this, you can login with the unit's *default
login credentials*, preferrably via a [console port][4].
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

Infix OS — Immutable.Friendly.Secure v25.01.0 (ttyS0)
example login:
```

Continued configuration is done as with any unit after factory reset.

[1]: netboot.md
[2]: https://github.com/kernelkit/infix/blob/main/src/confd/yang/confd/infix-system-software.yang
[3]: introduction.md#system-boot
[4]: management.md#console-port
[5]: scripting.md#-backup-configuration-using-sysrepocfg-and-scp

[^1]: In failure config, Infix puts all Ethernet ports as individual
    interfaces. With direct access, one can connect with e.g., SSH,
    using link local IPv6 addresses. This as an alternative to
    connecting via a console port.
[^2]: Set up an FTP/TFTP/SFTP or HTTP/HTTPS server on the same LAN.
