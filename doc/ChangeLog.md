Change Log
==========

All notable changes to the project are documented in this file.


[v23.10.0][] - 2023-10-31
-------------------------

> **Note:** upcoming releases will lock the `root` user for system-only
> services.  Instead an `admin` user will be the only default user with
> the CLI as its login shell.  This user is already available, so please
> consider updating any guidelines or documentation you may have.

### YANG Status

 - [ietf-system][]:
   - **augments:**
     - MotD (Message of the Day)
	 - User login shell, default: `/bin/false`
	 - State information for remotely querying firmware version information
   - **deviations:**
     - timezone-name, using IANA timezones instead of plain string
	 - UTC offset, only support per-hour offsets with [tzdata][]
	 - Usernames, clarifying Linux restrictions
     - Unsupported features marked as deviations, e.g. RADIUS
   - [infix-system-software][]: firmware upgrade with `install-bundle` RPC
 - [ietf-interfaces][]:
   - deviation to allow read-write `if:phys-address` for custom MAC address
   - [ietf-ip][]: augments
     - IPv4LL similar to standardized IPv6LL
   - [ietf-ip][]: deviations (`not-supported`) added for IPv4 and IPv6:
     - `/if:interfaces/if:interface/ip:ipv4/ip:address/ip:subnet/ip:netmask`
	 - `/if:interfaces/if:interface/ip:ipv6/ip:address/ip:status`
	 - `/if:interfaces/if:interface/ip:ipv4/ip:neighbor`
     - `/if:interfaces/if:interface/ip:ipv6/ip:neighbor`
   - ~~[ietf-if-vlan-encapsulation][]:~~ Removed in favor of a native model.
   - [infix-if-bridge][]: Linux bridge interfaces with native VLAN support
   - [infix-if-type][]: deviation for interface types, limiting number
     to supported types only.  New identities are derived from default
     IANA interface types, ensuring compatibility with other standard
     models, e.g., `ieee802-ethernet-interface.yang`
   - [infix-if-veth][]: Linux VETH pairs
   - [infix-if-vlan][]: Linux VLAN interfaces, e.g. `eth0.10` (New model!)
 - **Configurable services:**
   - [ieee802-dot1ab-lldp][]: stripped down to an `enabled` setting
   - [infix-services][]: support for enabling mDNS service/device discovery

[tzdata]:          https://www.iana.org/time-zones
[ieee802-dot1ab-lldp]: https://github.com/kernelkit/infix/tree/50a550b/src/confd/yang/ieee802-dot1ab-lldp%402022-03-15.yang
[ietf-system]:     https://www.rfc-editor.org/rfc/rfc7317.html
[ietf-interfaces]: https://www.rfc-editor.org/rfc/rfc7223.html
[ietf-ip]:         https://www.rfc-editor.org/rfc/rfc8344.html
[ietf-if-vlan-encapsulation]: https://www.ietf.org/id/draft-ietf-netmod-sub-intf-vlan-model-08.html
[infix-if-bridge]: https://github.com/kernelkit/infix/blob/fc5310b/src/confd/yang/infix-if-bridge%402023-08-21.yang
[infix-if-type]:   https://github.com/kernelkit/infix/tree/fc5310b/src/confd/yang/infix-if-type%402023-08-21.yang
[infix-if-veth]:   https://github.com/kernelkit/infix/tree/fc5310b/src/confd/yang/infix-if-veth%402023-06-05.yang
[infix-if-vlan]:   https://github.com/kernelkit/infix/blob/fc5310b/src/confd/yang/infix-if-vlan%402023-10-25.yang

[infix-ip]:        https://github.com/kernelkit/infix/tree/fc5310b/src/confd/yang/infix-ip%402023-09-14.yang
[infix-services]:  https://github.com/kernelkit/infix/blob/fc5310b/src/confd/yang/infix-services%402023-10-16.yang
[infix-system-software]: https://github.com/kernelkit/infix/tree/fc5310b/src/confd/yang/infix-system-software%402023-06-27.yang

### Changes

- Add support for setting/querying IPv4/IPv6 MTU, see #152 for details.
- Add support for *Fail Secure Mode*: if loading `startup-config` fails,
  e.g. YANG model validation failure after upgrade, the system now falls back
  to load `failure-config` instead of just crashing.  This config, along with
  `factory-config`, is generated on every boot to match the active image's
  YANG models.  In case neither config can be loaded, or even bootstrapping
  YANG models fail, the system will go into an RMA state -- Return to
  Manufacturer, clearly signaled on the console and, on devices that support
  it, angry LED signaling.  See #154 for more.
- Add support for generating GNS3 appliance file for NETCONF Aarch64.
- Add support for UTC offset (+/- HH:00) in `ietf-system`, PR #174
- Add support for `ietf-factory-default` RPC, PR #175
- Add support for performing factory reset (using #175 RPC) from CLI
- Replace `ietf-if-vlan-encapsulation` YANG model with the native
  `infix-if-vlan` model.  This fits better with Linux VLAN interfaces and
  simplifies the syntax greatly.  For details, see PR #179
  
        admin@example:/config/interfaces/interface/eth0.10/> set vlan id 10 lower-layer-if eth0

- The following new NETCONF interface operational counters have been added:
  
| **YANG**                    | **Linux / Ethtool**               |
|-----------------------------|-----------------------------------|
| `out-frames`                | `FramesTransmittedOK`             |
| `out-multicast-frames`      | `MulticastFramesXmittedOK`        |
| `out-broadcast-frames`      | `BroadcastFramesXmittedOK`        |
| `in-total-frames`           | `FramesReceivedOK`                |
|                             | + `FrameCheckSequenceErrors`      |
|                             | + `FramesLostDueToIntMACRcvError` |
|                             | + `AlignmentErrors`               |
|                             | + `etherStatsOversizePkts`        |
|                             | + `etherStatsJabbers`             |
| `in-frames`                 | `FramesReceivedOK`                |
| `in-multicast-frames`       | `MulticastFramesReceivedOK`       |
| `in-broadcast-frames`       | `BroadcastFramesReceivedOK`       |
| `in-error-undersize-frames` | `undersize_pkts`                  |
| `in-error-fcs-frames`       | `FrameCheckSequenceErrors`        |

- Greatly improved branding support using `make menuconfig`.  All the
  identifying strings, including firmware image, is in `/etc/os-release`, will
  be used in CLI `show system-information`, the WebUI About dialog, and any
  prominent areas when booting up (on console), logging in to CLI and WebUI.
- IGMP/MLD snooping is now disabled by default on new bridges.  Support
  for multicast filtering bridges expected no later than v24.01.
- The SSDP responder, device discovery in Windows, has been removed in favor
  of Windows 10 (build 1709) native support for mDNS-SD.  Details in #166
- A GreenPAK programmer has been added, not enabled by default.  This is a
  popular programmable little chip from Renesas.  Worth a look!
- The `confd` script `gen-interfaces` can now generate bridges and stand-alone
  interfaces with IPv6 (SLAAC) for `factory-config` et al.
- Drop `x86_64_minimal_defconfig`, previously used for regression tests only
- Documentation updates of how IPv4/IPv6 addresses are shown in NETCONF
  operational data, as well as the built-in CLI, see #163 for details.

### Fixes

- Fix #106: confd: drop deviation `ietf-system:timezone-utc-offset`
- Fix #151: Operational status broken in v23.09
- Fix #159: Hacky generation of `/etc/resolv.conf` at boot 
- Fix #162: VLAN interface without encapsulation is accepted by YANG model


[v23.09.0][] - 2023-10-02
-------------------------

> **Note:** upcoming releases will lock the `root` user for system-only
> services.  Instead an `admin` user will be the only default user with
> the CLI as its login shell.  This user is already available, so please
> consider updating any guidelines or documentation you may have.

### YANG Status

 - [ietf-system][]:
   - [infix-system][]: MotD (Message of the Day) augment
   - [infix-system][]: user login shell augment, default: `/bin/false`
   - [infix-system-software][]: system-state/software augment for
     remotely querying firmware version information
   - [infix-system-software][]: firmware upgrade with `install-bundle` RPC
   - [infix-system][]: timezone-name deviation, replaced with IANA timezones
   - [infix-system][]: username deviation, clarifying Linux restrictions
   - [infix-system][]: deviations for unsupported features, e.g. RADIUS
 - [ietf-interfaces][]:
   - [infix-interfaces][]: deviation for `if:phys-address` to allow read-write
   - [ietf-ip][]: augmented with IPv4LL similar to standardized IPv6LL
   - [infix-ip][]: deviations (`not-supported`) added for IPv4 and IPv6:
     - `/if:interfaces/if:interface/ip:ipv4/ip:address/ip:subnet/ip:netmask`
	 - `/if:interfaces/if:interface/ip:ipv6/ip:address/ip:status`
	 - `/if:interfaces/if:interface/ip:ipv4/ip:mtu`
	 - `/if:interfaces/if:interface/ip:ipv6/ip:mtu`
	 - `/if:interfaces/if:interface/ip:ipv4/ip:neighbor`
     - `/if:interfaces/if:interface/ip:ipv6/ip:neighbor`
   - [ietf-if-vlan-encapsulation][]: Linux VLAN interfaces, e.g. `eth0.10`
   - [infix-if-bridge][]: Linux bridge interfaces with native VLAN support
   - [infix-if-veth][]: Linux VETH pairs
   - [infix-if-type][]: deviation for interface types, limiting number
     to supported types only.  New identities are derived from default
     IANA interface types, ensuring compatibility with other standard
     models, e.g., `ieee802-ethernet-interface.yang`
 - Configurable services:
   - [ieee802-dot1ab-lldp][]: stripped down to an `enabled` setting
   - [infix-services][]: support for enabling mDNS and SSDP discovery

[br2023.02.2]:     https://git.busybox.net/buildroot/tag/?h=2023.02.2
[ieee802-dot1ab-lldp]: https://github.com/kernelkit/infix/tree/50a550b/src/confd/yang/ieee802-dot1ab-lldp%402022-03-15.yang
[ietf-system]:     https://www.rfc-editor.org/rfc/rfc7317.html
[ietf-interfaces]: https://www.rfc-editor.org/rfc/rfc7223.html
[ietf-ip]:         https://www.rfc-editor.org/rfc/rfc8344.html
[ietf-if-vlan-encapsulation]: https://www.ietf.org/id/draft-ietf-netmod-sub-intf-vlan-model-08.html
[infix-if-bridge]: https://github.com/kernelkit/infix/tree/784c175/src/confd/yang/infix-if-bridge%402023-08-21.yang
[infix-if-type]:   https://github.com/kernelkit/infix/tree/784c175/src/confd/yang/infix-if-type%402023-08-21.yang
[infix-if-veth]:   https://github.com/kernelkit/infix/tree/784c175/src/confd/yang/infix-if-veth%402023-06-05.yang
[infix-interfaces]: https://github.com/kernelkit/infix/tree/784c175/src/confd/yang/infix-interfaces%402023-09-19.yang
[infix-ip]:        https://github.com/kernelkit/infix/tree/784c175/src/confd/yang/infix-ip%402023-09-14.yang
[infix-services]:  https://github.com/kernelkit/infix/tree/784c175/src/confd/yang/infix-services%402023-08-22.yang
[infix-system]:    https://github.com/kernelkit/infix/tree/784c175/src/confd/yang/infix-system%402023-08-15.yang
[infix-system-software]: https://github.com/kernelkit/infix/tree/784c175/src/confd/yang/infix-system-software%402023-06-27.yang

### Changes

- The following new NETCONF interface operational status have been added:
  - admin-status
  - IP address origin (dhcp, static, link-layer, random, other)
  - bridge
  - parent-interface
  - basic statistics (`in_octets`, `out_octets`)
- Support for custom interface `phys-address` (MAC address)
- The CLI admin-exec command `show interfaces` now fully uses NETCONF
  operational data to display both available interfaces and all of their
  IP addresses.  Displaying an individual interface will show more info.
- The CLI admin-exec command `password encrypt` now default to SHA512

### Fixes

- Fix #136: IPv6 autoconf `create-global-addresses true` does not bite
- Fix #138: Not possible to have static IP and DHCP at the same time
- Minor fixes and updates to documentation (faulty links, references)
- The `sync-fork.yml` workflow has finally been fixed.


[v23.08.0][] - 2023-08-31
-------------------------

> **Note:** upcoming releases will lock the `root` user for system-only
> services.  Instead an `admin` user will be the only default user with
> the CLI as its login shell.  This user is already available, so please
> consider updating any guidelines or documentation you may have.

### YANG Status

 - [ietf-system][]:
   - [infix-system][]: MotD (Message of the Day) augment
   - [infix-system][]: user login shell augment, default: `/bin/false`
   - [infix-system-software][]: system-state/software augment for
     remotely querying firmware version information
   - [infix-system-software][]: firmware upgrade with `install-bundle` RPC
   - [infix-system][]: timezone-name deviation, replaced with IANA timezones
   - [infix-system][]: username deviation, clarifying Linux restrictions
   - [infix-system][]: deviations for unsupported features, e.g. RADIUS
 - [ietf-interfaces][]:
   - [ietf-ip][]: augmented with IPv4LL similar to standardized IPv6LL
   - [ietf-if-vlan-encapsulation][]: Linux VLAN interfaces, e.g. `eth0.10`
   - [infix-if-bridge][]: Linux bridge interfaces with native VLAN support
   - [infix-if-veth][]: Linux VETH pairs
   - [infix-if-type][]: deviation for interface types, limiting number
     to supported types only.  New identities are derived from default
     IANA interface types, ensuring compatibility with other standard
     models, e.g., `ieee802-ethernet-interface.yang`
 - Configurable services:
   - [ieee802-dot1ab-lldp][]: stripped down to an `enabled` setting
   - [infix-services][]: support for enabling mDNS and SSDP discovery

[br2023.02.2]:     https://git.busybox.net/buildroot/tag/?h=2023.02.2
[ieee802-dot1ab-lldp]: https://github.com/kernelkit/infix/tree/50a550b/src/confd/yang/ieee802-dot1ab-lldp%402022-03-15.yang
[ietf-system]:     https://www.rfc-editor.org/rfc/rfc7317.html
[ietf-interfaces]: https://www.rfc-editor.org/rfc/rfc7223.html
[ietf-ip]:         https://www.rfc-editor.org/rfc/rfc8344.html
[ietf-if-vlan-encapsulation]: https://www.ietf.org/id/draft-ietf-netmod-sub-intf-vlan-model-08.html
[infix-if-bridge]: https://github.com/kernelkit/infix/tree/50a550b/src/confd/yang/infix-if-bridge%402023-08-21.yang
[infix-if-veth]:   https://github.com/kernelkit/infix/tree/50a550b/src/confd/yang/infix-if-veth%402023-06-05.yang
[infix-if-type]:   https://github.com/kernelkit/infix/tree/50a550b/src/confd/yang/infix-if-type%402023-08-21.yang
[infix-services]:  https://github.com/kernelkit/infix/tree/50a550b/src/confd/yang/infix-services%402023-08-22.yang
[infix-system]:    https://github.com/kernelkit/infix/tree/50a550b/src/confd/yang/infix-system%402023-08-15.yang
[infix-system-software]: https://github.com/kernelkit/infix/tree/50a550b/src/confd/yang/infix-system-software%402023-06-27.yang

### Changes

 - Bump Linux kernel: v5.19 to v6.1
 - Updated board support for Microchip SparX-5i and Marvell CN9130 CRB
 - New logo and significant updates to the documentation
 - New NETCONF RPC `factory-default` to reset `running-config`
 - Replaced limited BusyBox ping with iputils-ping
 - Most system services are now disabled by default, support for enabling
   LLDP, mDNS-SD, and SSDP using NETCONF, enabled in `factory-config`
 - Firmware upgrade framework, based on [RAUC](https://rauc.io/), added
   - Matching YANG model (see above) and an `install-bundle` RPC
   - Currently supported upgrade protocols: HTTP/HTTPS, FTP, SCP
 - Initial support for interface operational status, in ietf-interfaces
 - Add support for setting user login shell: `bash`, `clish`, `false`
 - Default login shell for new users: `false`
 - Massive updates and fixes to the CLI (klish):
   - Line editing now works as similar CLI:s from major vendors
   - Hotkey fixes: Ctrl-D and Ctrl-Z now work as expected
   - Prompt changed from JunOS style to be more similar to Bash
   - Online help commands, both in admin-exec and configure context,
     type `help` after entering the CLI to get started
   - Improved help for configure context using YANG descriptions
   - Support for reading and setting system "datetime" (RPC), an
     optional `iso` keyword can be used when reading time to see the
     format required when setting the time
   - Support for showing interfaces status, using above operational data
   - Support for showing bridge status: links, fdb, mdb, vlans
   - Support for showing log files, including tailing with `follow`
   - Support for `password generate` and `password encrypt`, highly
     useful from configure context when creating new users: use the
	 `do password encrypt type sha256` to generate the hash
   - Support show uptime, version, calling `netcalc`, ping, and tcpdump

### Fixes

 - Fix #57: unneccesary lldpd restarts on configuration change
 - Ensure mDNS advertises the correct hostname after hostname change
 - Fix regression in enabling IPv4 ZeroConf address
 - Loopback interface now shows `UP` operstate instead of `UNKNOWN`
 - Fix adding user without password, i.e., login using SSH keys only


[v23.06.0][] - 2023-06-23
-------------------------

Midsummer release.  The first official Infix release, based on [Buildroot
2023.02.2][br2023.02.2], with NETCONF support using the [sysrepo][] and
[netopeer2][].

Supported YANG models in addition to those used by sysrepo and netopeer:

 - [ietf-system][]
   - Everything except radius authentication and timezone-utc-offset
   - Augmented with MotD (Message of the Day), Infix YANG model
 - [ietf-interfaces][]
   - [ietf-ip][] augmented with IPv4LL similar to standardized IPv6LL
   - [ietf-if-vlan-encapsulation][], Linux VLAN interfaces, e.g. `eth0.10`
   - Linux bridge interfaces with native VLAN support, Infix YANG model
   - Linux VETH pairs, Infix YANG model

> **DISCLAIMER:** the [Infix YANG models for Linux][yang23.06], are
> still under heavy development.  New revisions are expected which may
> not be backwards compatible.  When upgrading to a new release, test on
> a GNS3 staging environment first, or start over from a factory reset.

[br2023.02.2]:     https://git.busybox.net/buildroot/tag/?h=2023.02.2
[ietf-system]:     https://www.rfc-editor.org/rfc/rfc7317.html
[ietf-interfaces]: https://www.rfc-editor.org/rfc/rfc7223.html
[ietf-ip]:         https://www.rfc-editor.org/rfc/rfc8344.html
[ietf-if-vlan-encapsulation]: https://www.ietf.org/id/draft-ietf-netmod-sub-intf-vlan-model-08.html
[yang23.06]:       https://github.com/kernelkit/infix/blob/aea74842e0475441d8df834f2dcd8cbc21fa253d/src/confd/yang/infix-interfaces%402023-06-05.yang

### Changes

 - Bump sysrepo to v2.2.73
   - Backport support for initializing factory-default data store with
     default config data also for sysrepo internal modules.
   - Add support for `sysrepocfg -Cfactory -d running`, for testing
 - Bump netopeer2 to v2.1.62
 - Bump libyang to v2.1.80
 - Add klish, a CLI for sysrepo
 - Add podman for container support, backported from upstream Buildroot
 - Add conmon for container support, backported from upstream Buildroot
 - Backport cni-plugins support for host-local and static plugins

### Fixes

 - N/A

[buildroot]:  https://buildroot.org/
[UNRELEASED]: https://github.com/kernelkit/infix/compare/v23.10.0...HEAD
[v23.10.0]:   https://github.com/kernelkit/infix/compare/v23.09.0...v23.10.0
[v23.09.0]:   https://github.com/kernelkit/infix/compare/v23.08.0...v23.09.0
[v23.08.0]:   https://github.com/kernelkit/infix/compare/v23.06.0...v23.08.0
[v23.06.0]:   https://github.com/kernelkit/infix/compare/BASE...v23.06.0
[sysrepo]:    https://www.sysrepo.org/
[netopeer2]:  https://github.com/CESNET/netopeer2
