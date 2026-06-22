# Upgrading & Boot Order

For resilience purposes, Infix maintains two software images referred to
as the _primary_ and _secondary_ partition image.  In addition, some
bootloaders support [netbooting][1].

The _boot order_ defines which image is tried first, and is listed with
the CLI `show software` command. It also shows Infix version installed
per partition, and which image was used when booting (`STATE booted`).

<pre class="cli"><code>admin@example:/> <b>show software</b>
Boot order : primary secondary net

<span class="header">NAME       STATE     VERSION                DATE                     </span>
primary    booted    v25.01.0               2025-04-25T10:15:00+00:00
secondary  inactive  v25.01.0               2025-04-25T10:07:20+00:00
admin@example:/>
</code></pre>

YANG support for upgrading Infix, inspecting and _modifying_ the
boot-order, is defined in [infix-system-software][2].


## Changing Boot Order

The boot order can be manually changed using the `set boot-order` command.
This is useful for rolling back to a previous version or changing the
preferred boot source.

The command accepts one to three boot targets as separate arguments, in the
desired boot order. Valid boot targets are:

- `primary` - The primary partition
- `secondary` - The secondary partition
- `net` - Network boot (if supported by bootloader)

The CLI provides tab-completion for boot targets, making it easy to enter
valid values.

Example: View current boot order and change it:

<pre class="cli"><code>admin@example:/> <b>show boot-order</b>
primary secondary net
admin@example:/> <b>set boot-order secondary primary net</b>
admin@example:/> <b>show boot-order</b>
secondary primary net
admin@example:/>
</code></pre>

Example: Set boot order to only try primary partition:

<pre class="cli"><code>admin@example:/> <b>show boot-order</b>
secondary primary net
admin@example:/> <b>set boot-order primary</b>
admin@example:/> <b>show boot-order</b>
primary
admin@example:/>
</code></pre>

Example: Using tab-completion (press TAB to see available options):

<pre class="cli"><code>admin@example:/> <b>set boot-order </b><kbd>TAB</kbd>
net        primary    secondary
admin@example:/> <b>set boot-order secondary </b><kbd>TAB</kbd>
net        primary    secondary
admin@example:/> <b>set boot-order secondary primary</b>
admin@example:/> <b>show boot-order</b>
secondary primary
admin@example:/>
</code></pre>

The new boot order takes effect on the next reboot and can be verified
with `show boot-order` or `show software`:

<pre class="cli"><code>admin@example:/> <b>show software</b>
Boot order : secondary primary

<span class="header">NAME       STATE     VERSION                DATE                     </span>
primary    booted    v25.01.0               2025-04-25T10:15:00+00:00
secondary  inactive  v25.01.0               2025-04-25T10:07:20+00:00
admin@example:/>
</code></pre>

> [!NOTE]
> The boot order is automatically updated when performing an upgrade.
> The newly installed image will be set as the first boot target.
>
> Duplicate boot targets are not allowed. The CLI will reject attempts to
> specify the same target multiple times.


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

<pre class="cli"><code>admin@example:/> <b>dir /cfg</b>
/cfg directory
backup/             ssl/                startup-config.cfg

admin@example:/> <b>copy /cfg/startup-config.cfg /cfg/v25.01.0-startup-config.cfg</b>
admin@example:/> <b>dir /cfg</b>
/cfg directory
backup/             ssl/                startup-config.cfg           v25.01.0-startup-config.cfg

admin@example:/>
</code></pre>

*Upgrade:* Here the image *pkg bundle* was made available via TFTP.

<pre class="cli"><code>admin@example:/> <b>upgrade tftp://198.18.117.1/infix-aarch64-25.03.1.pkg</b>
installing
  0% Installing
  0% Determining slot states
 10% Determining slot states done.
...
 40% Checking slot rootfs.1 (secondary)
 46% Checking slot rootfs.1 (secondary) done.
...
 98% Copying image to rootfs.1
 99% Copying image to rootfs.1
 99% Copying image to rootfs.1 done.
 99% Updating slots done.
100% Installing done.
Installing `tftp://198.18.117.1/infix-aarch64-25.03.1.pkg` succeeded
admin@example:/>
</code></pre>

*Reboot:* The unit will boot on the other partition, with the newly
installed image. The `Loading startup-config` step conducts migration
of startup configuration if applicable.

<pre class="cli"><code>admin@example:/> <b>reboot</b>
[ OK ] Stopping Static routing daemon
[ OK ] Stopping Zebra routing daemon
...
[ OK ] Loading startup-config
[ OK ] Verifying self-signed https certificate
[ OK ] Update DNS configuration
[ OK ] Starting Status daemon

Infix OS — Immutable.Friendly.Secure v25.03.1 (ttyS0)
example login: <b>admin</b>
Password:
.-------.
|  . .  | Infix OS — Immutable.Friendly.Secure
|-. v .-| https://www.kernelkit.org
'-'---'-'

Run the command 'cli' for interactive OAM

admin@example:~$ <b>cli</b>

See the 'help' command for an introduction to the system

admin@example:/> <b>show software</b>
Boot order : secondary primary net

<span class="header">NAME       STATE     VERSION                DATE                     </span>
primary    inactive  v25.01.0               2025-04-25T10:15:00+00:00
secondary  booted    v25.03.1               2025-04-25T10:24:31+00:00
admin@example:/>
</code></pre>

As shown, the *boot order* has been updated, so that *secondary* is
now the preferred boot source.

To upgrade the remaining partition (`primary`), run the `upgrade URL`
command again, and (optionally) reboot.

## Unattended Updates

The upgrade above is operator-driven: you pick a bundle, run `upgrade`,
and reboot.  Infix can also perform that same install on its own, on a
recurring [schedule][6].

Two independent features share one update source:

- **Update checks** (`check-update`) look for a newer release and log a
  notification, shown on the next login.  Nothing is downloaded or
  installed
- **Unattended updates** (`unattended-update`) also download and install
  the new release, exactly as a manual `upgrade` would

### Update Source

Both features read the same `update-url`, which points at an RSS/Atom feed
of releases.  It defaults to the Infix project's release feed:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>set system software update-url https://github.com/kernelkit/infix/releases.atom</b>
admin@example:/config/> <b>set system software allow-prerelease false</b>
admin@example:/config/> <b>leave</b>
</code></pre>

The newest entry the feed offers decides the latest version.  Each entry
must link to its release page as `<base>/releases/tag/<tag>`, and that is
where the version tag comes from.  Override `update-url` to follow a fork
or a customer-specific release channel.

Feeds commonly list release candidates alongside finished releases.  By
default those are ignored, so only a final release is ever installed; set
`allow-prerelease` to `true` to consider them.

A feed carries no asset list, so the per-platform bundle is fetched by
convention from:

```
<base>/releases/download/<tag>/<image-id>-<tag>.pkg
```

where `<image-id>` is the running system's `IMAGE_ID`, e.g.
`infix-aarch64`.  RAUC streams the bundle straight from that URL.  Nothing
is staged on disk first, so the update needs no free space for the image,
but the server must support HTTP range requests.

### Hosting Your Own Feed

Any static web server will do.  The feed and the bundles are plain files,
and the device fetches the feed, then the `.pkg` whose URL it derives from
the feed.

Atom and RSS 2.0 both work.  An Atom feed carries one `<entry>` per
release, each with a `<link>` whose `href` ends in `/releases/tag/<tag>`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example Infix releases</title>
  <entry>
    <title>v26.08.1</title>
    <updated>2026-08-20T10:00:00Z</updated>
    <link href="https://releases.example.com/infix/releases/tag/v26.08.1"/>
  </entry>
  <entry>
    <title>v26.05.0</title>
    <updated>2026-05-14T10:00:00Z</updated>
    <link href="https://releases.example.com/infix/releases/tag/v26.05.0"/>
  </entry>
</feed>
```

An RSS 2.0 feed carries the same URLs, as the text of an `<item>` element's
`<link>` rather than an attribute:

```xml
<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <item>
      <title>v26.08.1</title>
      <link>https://releases.example.com/infix/releases/tag/v26.08.1</link>
    </item>
  </channel>
</rss>
```

Everything before `/releases/tag/` in that URL becomes the base URL, so
the example above resolves bundles under
`https://releases.example.com/infix/releases/download/<tag>/`.  Lay the
files out to match, naming the feed whatever `update-url` points at:

```
infix/
├── releases.atom
└── releases
    └── download
        └── v26.08.1
            ├── infix-aarch64-v26.08.1.pkg
            └── infix-x86_64-v26.08.1.pkg
```

Then point the device at the feed:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>set system software update-url https://releases.example.com/infix/releases.atom</b>
admin@example:/config/> <b>leave</b>
</code></pre>

**Requirements:**

- **Atom or RSS 2.0.**  Atom is tried first, reading the `href` attribute
  of each entry's `link`.  When that finds nothing, the URLs are read from
  the text of each RSS item's `link` instead
- **Newest entry first.**  Selection follows feed order, so the first
  entry that passes the pre-release filter wins.  A feed listing releases
  oldest-first offers the oldest release
- **The tag is the last path segment** of the release URL, and it goes
  verbatim into the bundle filename.  A tag containing `-rc`, `-alpha` or
  `-beta` counts as a pre-release, which is skipped unless
  `allow-prerelease` is `true`
- **One bundle per platform**, named `<image-id>-<tag>.pkg`.  A device
  looks only for its own `IMAGE_ID`, so one feed can serve several
  platforms
- **HTTP range requests.**  RAUC streams the bundle instead of downloading
  it whole, so a server that ignores `Range` fails the install.  BusyBox
  `httpd` and nginx both work; Python's `http.server` does not

Infix uses the `/releases/tag/<tag>` URL only to derive the base and as a
human-readable link in the update-check notification.  The page itself
does not have to exist.

> [!TIP]
> Serving the feed over HTTPS requires a correct clock on the device, or
> certificate validation fails and every occurrence is skipped.  Plain
> HTTP avoids that on an isolated network.

### Enabling Unattended Updates

Unattended updates are off by default and need a [schedule][6] to trigger
them.  The example below installs new releases during a nightly
maintenance window, leaving the reboot to the operator.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>set system schedule nightly recurrence frequency daily</b>
admin@example:/config/> <b>set system schedule nightly recurrence byhour 3</b>
admin@example:/config/> <b>set system software unattended-update enabled true</b>
admin@example:/config/> <b>set system software unattended-update schedule nightly</b>
admin@example:/config/> <b>set system software unattended-update reboot manual</b>
admin@example:/config/> <b>leave</b>
</code></pre>

**Parameters:**

- `enabled`: Enable unattended updates (default: `false`).  Without a
  referenced schedule no updates are performed either way
- `schedule`: The [schedule][6] whose occurrences trigger an update
- `reboot`: What to do after a successful install
    - `manual` (default): Install and flip the boot-order, but do not
      reboot.  The new image activates the next time the operator reboots
    - `immediate`: Reboot automatically to activate the new image at once

### What Happens on Each Occurrence

1. The feed is queried for the latest release.  If it cannot be reached,
   the occurrence is logged and skipped, and the job exits successfully
2. If the latest release is not newer than the running version, nothing
   happens
3. Otherwise the platform bundle is installed to the *inactive* partition,
   and the boot-order is flipped to activate it on the next boot.  The
   partition currently running is left untouched as a fallback
4. Depending on the `reboot` policy, the system either reboots or logs
   that a reboot is needed

A single-instance lock means occurrences never overlap: if an install is
still running when the next one fires, the new occurrence is skipped.

> [!IMPORTANT]
> Rollback safety is identical to a manual `upgrade`: the previously
> running image remains on the other partition, and the bootloader falls
> back to it if the new image does not boot.  Infix does no health check
> on the new image beyond that, so see the caution under
> [Upgrading](#upgrading) about upgrading only one partition at a time.

### Monitoring

Operator-facing messages go to `/var/log/messages`, while skipped
occurrences are logged at `daemon.info`/`daemon.debug` in
`/var/log/syslog`:

```sh
admin@example:~$ grep unattended-update /var/log/messages
unattended-update: Installing v26.08.1 from https://.../infix-aarch64-v26.08.1.pkg (running v26.05.0)
unattended-update: Installed v26.08.1; reboot to activate the new image
```

| Message                                      | Meaning                          |
|----------------------------------------------|----------------------------------|
| `Installing <tag> from <url> (running <ver>)`| Install started                  |
| `Installed <tag>; reboot to activate …`      | Success, `reboot manual`         |
| `No update available (current: …, latest: …)`| Ran, nothing to do               |
| `Skipped: failed to query latest release …`  | Feed unreachable                 |
| `Another update is already in progress …`    | Previous occurrence still running|

`show software` reports installation state, slot contents and the boot
order, both during and after the install.

> [!TIP]
> A system running a development build has no comparable version number
> and is always considered upgradable, so an unattended update on a dev
> build installs the latest release from the feed on the first occurrence.

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

<pre class="cli"><code>admin@example:/> <b>dir /cfg/backup/</b>
/cfg/backup/ directory
startup-config-1.4.cfg

admin@example:/>
</code></pre>

The modifications made to the startup configuration can be viewed by
comparing the files from the *shell*. An example is shown below.

<pre class="cli"><code>admin@example:/> <b>exit</b>
admin@example:~$ <b>diff /cfg/backup/startup-config-1.4.cfg /cfg/startup-config.cfg</b>
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
</code></pre>

## Downgrading

Downgrading to an earlier version is possible, however, downgrading is
**not** guaranteed to work smoothly.  In particular, when the unit boots
up with the downgraded version, it may fail to apply the *startup
config*, and instead apply its [failure config][3].

We consider two cases: downgrading with and without applying a backup
startup configuration before rebooting.

In both cases we start out with a unit running Infix v25.03.1, and
wish to downgrade to v25.01.0.

<pre class="cli"><code>admin@example:/> <b>show software</b>
Boot order : primary secondary net

<span class="header">NAME       STATE     VERSION                DATE                     </span>
primary    booted    v25.03.1               2025-04-25T11:36:26+00:00
secondary  inactive  v25.03.1               2025-04-25T10:24:31+00:00
admin@example:/>
</code></pre>

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

<pre class="cli"><code>admin@example:/> <b>dir /cfg</b>
/cfg directory
backup/       ssl/       startup-config.cfg    v25.01.0-startup-config.cfg

admin@example:/>
</code></pre>

The alternative is to use a startup config implicitly backed up by the
system as part of [Configuration Migration](#configuration-migration).

<pre class="cli"><code>admin@example:/> <b>dir /cfg/backup/</b>
/cfg/backup/ directory
startup-config-1.4.cfg

admin@example:/>
</code></pre>

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

<pre class="cli"><code>admin@example:/> <b>upgrade tftp://198.18.117.1/infix-aarch64-25.01.0.pkg</b>
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
</code></pre>

*Apply the backup configuration file:*

It is recommended to use a backup configuration file for the Infix version to
downgrade to, if there is one available.

<pre class="cli"><code>admin@example:/> <b>copy /cfg/v25.01.0-startup-config.cfg /cfg/startup-config.cfg</b>
Overwrite existing file /cfg/startup-config.cfg (y/N)? <b>y</b>
admin@example:/>
</code></pre>

An alternative is to use a backup file stored when the system
conducted a [configuration migration](#configuration-migration). See
the *caution* note above.

<pre class="cli"><code>admin@example:/> <b>copy /cfg/backup/startup-config-1.4.cfg /cfg/startup-config.cfg</b>
Overwrite existing file /cfg/startup-config.cfg (y/N)? <b>y</b>
admin@example:/>
</code></pre>

*Reboot:*

The unit will come up with the applied backup configuration.

<pre class="cli"><code>admin@example:/> <b>reboot</b>
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
</code></pre>

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

<pre class="cli"><code>admin@example:/> <b>upgrade tftp://198.18.117.1/infix-aarch64-25.01.0.pkg</b>
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
</code></pre>

*Reboot:*

Conduct a reboot. During boot, the unit fails to apply the existing
startup configuration (config version `1.5` while software expects
version `1.4` or earlier), and instead applies its [failure
config][3]. This is what is seen on the console when this situation
occurs. Note that the login prompt displays `failed` as part of the
*hostname*.

<pre class="cli"><code>admin@example:/> <b>reboot</b>
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
</code></pre>

To remedy a situation like this, you can login with the unit's *default
login credentials*, preferrably via a [console port][4].
The unit's default credentials are typically printed on a sticker on
the unit.

<pre class="cli"><code>failed-00-00-00 login: <b>admin</b>
Password:

Run the command 'cli' for interactive OAM

admin@failed-00-00-00:~$
</code></pre>

When it is *safe* from a network operations perspective, you can
conduct a factory reset and reboot. It is recommended to remove the
unit from any production network before doing this, as a factory reset
may enable undesired connectivity between the unit's ports.

<pre class="cli"><code>admin@failed-00-00-00:~$ <b>factory</b>
Factory reset device (y/N)? <b>y</b>
factory: scheduled factory reset on next boot.
Reboot now to perform reset, (y/N)? <b>y</b>
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
</code></pre>

Continued configuration is done as with any unit after factory reset.

[1]: netboot.md
[2]: https://github.com/kernelkit/infix/blob/main/src/confd/yang/confd/infix-system-software.yang
[3]: boot.md#system-boot
[4]: management.md#console-port
[5]: scripting.md#-backup-configuration-using-sysrepocfg-and-scp
[6]: schedule.md

[^1]: In failure config, Infix puts all Ethernet ports as individual
    interfaces. With direct access, one can connect with e.g., SSH,
    using link local IPv6 addresses. This as an alternative to
    connecting via a console port.
[^2]: Set up an FTP/TFTP/SFTP or HTTP/HTTPS server on the same LAN.
