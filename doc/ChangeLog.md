Change Log
==========

All notable changes to the project are documented in this file.


[v24.03.0][UNRELEASED]
-------------------------

Please note, as of this release the Infix Classic variant has been
dropped.  This was the legacy, pre-NETCONF, Infix with manual config of
the system using a persistent `/etc`.  It may be resurrected later as a
separate project.  Going forward Infix' focus is entirely on NETCONF.

> Development in progress, for daily updates see the team board:
> <https://github.com/orgs/kernelkit/projects/3/views/2>

### Changes
- Bump the base Linux kernel version to 6.6
- Drop Classic variant to reduce overhead, simplify build & release
  processes, and focus on NETCONF for Arm64 and Amd64 platforms
- Add hostname restrictions to ietf-system, and infix-dhcp-client
  models.  Max 64 characters on Linux systems
- The bridge model now has built-in validation of port memberships,
  i.e., a port must be a bridge member to be used in VLAN filtering
- The bridge model only permits the bridge itself to be a tagged
  member of VLANs -- meaning, the only way to set an IP address on
  such bridges is to use a VLAN interface on top
- A VLAN filtering bridge now validates that no IP address has been
  set.  Use a VLAN interface on top for that (see above)
- Container documentation: CLI prompts have been updated to match the
  examples used in other parts of the User Guide
- Issue #358: translate YANG model's LOWER-LAYER-DOWN -> LINK-DOWN in
  CLI `show interfaces` command
- Issue #360: document factory-config, startup-config, and the various
  failure modes in the system
- Issue #361: document how a privileged container can break out of its
  confinement and run host commands, e.g., call `sysrepocfg`
- Issue #365: add limited support for container capabilities, e.g., to
  enable `CAP_NET_RAW` to allow containers to use `ping`.  This allows
  users to avoid enabling privileged mode
- Issue #367: setting date/time over NETCONF now saves system time also
  to the RTC, which otherwise is only saved on reboot or power-down
- Add support for static multicast filters, MAC, IPv4 and IPv6 groups
  are supported.
- Include Buildroot `legal-info` in releases, i.e., licenses, sources
  with patches, as well as csv files for packages and toolchain
- Issue #369: Remove limitation that the routing instance must be
  named 'default'

### Fixes
- Issue #391 Creating VLAN interface in the CLI with "edit interface vlanN"
  does not set VLAN id to N.
- confd: Fix memory leak when operating on candidate configuration.
- CLI: fix VLAN inference for interfaces named `eth0.1`, i.e., VID 1 on
  lower-layer-if `eth0`.  Only affects automatic inference in the CLI,
  entering the values manually (CLI/NETCONF) not affected by this bug
- Reduced syslog errors for accesses no non-existing xpaths
- Fix bogus warning about not properly updating `/etc/motd` in new
  `motd-banner` setting, introduced in v24.02.0
- infix-routing model: the `enable` configuration setting for OSPF, in
  `default-route-advertise` has been obsoleted and replaced by `enabled`
- Fix #328: when setting up a VLAN filtering bridge, the PVID for bridge
  ports defaulted to 1, making it impossible to set up "tagged-only"
  ports which drop ingressing untagged traffic
- Fix #358: MAC address no longer shown for bridge interfaces in CLI
  `show interfaces` command
- Fix #366: static routes from container host interfaces do not work.
  Documentation updated with an example
- Fix #368: upgrading `oci-archive:/` images fail because system thinks
  the image can be pulled from a localhost registry.  Documentation has
  also been updated, describing various methods and how to upgrade them
- Fix #370: despite the documentation stating containers must explicitly
  declare `network` settings, Infix v23.02 had a late regression that
  reverted back to the podman default: network behind a CNI bridge
  (firewalled and NAT:ed, hidden from the rest of the network)
- Fix #385: segfault in helper function when disabling the DHCP client
  using `no dhcp-client` from the CLI


[v24.02.0][] - 2024-03-01
-------------------------

> **Note:** the `root` account is disabled in official builds.  Only the
> `admin` user can log in to the system.  This can be changed, but only
> in developer builds: `make menuconfig` -> System configuration ->
> `[*]Enable root login with password`

### YANG Status

Infix devices support downloading all YANG models over NETCONF, including
models with submodules.  As a rule, standard models are used as long as
they map to underlying Linux concepts and services.  All exceptions are
listed in Infix specific models, detailing deviations and augmentations.

Currently supported models:

 - [ieee802-ethernet-interface][]:
   - Toggle port speed & duplex auto-negotiation on/off
   - Set port speed and duplex when auto-negotiation is off
   - Query port speed/duplex and auto-negotiation status (operational)
   - **Frame counters:**

    | **YANG**                    | **Linux / Ethtool**               |
    |-----------------------------|-----------------------------------|
    | `out-frames`                | `FramesTransmittedOK`             |
    | `out-multicast-frames`      | `MulticastFramesXmittedOK`        |
    | `out-broadcast-frames`      | `BroadcastFramesXmittedOK`        |
    | `in-total-octets`           | `FramesReceivedOK`                |
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
    | `in-good-octets`            | `OctetsReceivedOK`                |
    | `out-good-octets`           | `OctetsTransmittedOK`             |

 - [ietf-hardware][]:
   - Populates standard hardware model from corresponding data in device EEPROMs
   - **augments:**
     - Initial support for USB ports
     - Vital Product Data (VPD) from device EEPROMs ([ONIE][] structure)
   - [infix-hardware][]: Deviations and augments
 - [ietf-system][]:
   - **augments:**
     - Message of the Day (MotD) banner, shown after SSH or console login.
	   Please note: the legacy `motd` has been replaced with `motd-banner` os
	   of v24.02.  Use CLI `text-editor` to modify the latter
	 - User login shell, default: `/bin/false` (no SSH or console login)
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
   - [ietf-routing][]: Base model for routing
   - [ietf-ipv4-unicast-routing][]: Static unicast routing, incl. operational
     data, i.e., setting static IPv4 routes and reading IPv4 routing table
   - [ietf-ipv6-unicast-routing][]: Static unicast routing, incl. operational
     data, i.e., setting static IPv6 routes and reading IPv6 routing table
   - [ietf-ospf][]: Limited support for OSPFv2, with additional support for
     injecting default route, and route redistribution.  Underlying routing
     engine in use is Frr.  Includes operational status + data (routes).
	 See [infix-routing][] model for detailed list of deviations
   - [infix-ethernet-interface][]: deviations for ieee802-ethernet-interface
   - [infix-routing][]: Limit ietf-routing to one instance `default` per
     routing protocol, also details unsupported features (deviations) to both
     ietf-routing and ietf-ospf models, as well as augments made to support
     injecting default route in OSPFv2
   - [infix-if-bridge][]: Linux bridge interfaces with native VLAN support
   - [infix-if-type][]: deviation for interface types, limiting number to
     supported types only.  New identities are derived from default IANA
     interface types, ensuring compatibility with other standard models, e.g.,
     `ieee802-ethernet-interface.yang`
   - [infix-if-veth][]: Linux VETH pairs
   - [infix-if-vlan][]: Linux VLAN interfaces, e.g. `eth0.10`
 - [infix-containers][]: Support for Docker containers, incl. operational data
   to query status and remotely stop/start containers
 - [infix-dhcp-client][]: DHCPv4 client, including supported options
 - **Configurable services:**
   - [ieee802-dot1ab-lldp][]: stripped down to an `enabled` setting
   - [infix-services][]: support for enabling mDNS service/device discovery

[tzdata]:          https://www.iana.org/time-zones
[ietf-system]:     https://www.rfc-editor.org/rfc/rfc7317.html
[ietf-interfaces]: https://www.rfc-editor.org/rfc/rfc7223.html
[ietf-ip]:         https://www.rfc-editor.org/rfc/rfc8344.html
[ietf-if-vlan-encapsulation]: https://www.ietf.org/id/draft-ietf-netmod-sub-intf-vlan-model-08.html
[ietf-routing]:     https://www.rfc-editor.org/rfc/rfc8349
[ietf-ipv4-unicast-routing]: https://www.rfc-editor.org/rfc/rfc8349#page-29
[ietf-ipv6-unicast-routing]: https://www.rfc-editor.org/rfc/rfc8349#page-37
[ietf-hardware]:    https://www.rfc-editor.org/rfc/rfc8348
[ietf-ospf]:        https://www.rfc-editor.org/rfc/rfc9129
[ieee802-dot1ab-lldp]: https://github.com/kernelkit/infix/blob/f0c23ca/src/confd/yang/ieee802-dot1ab-lldp%402022-03-15.yang
[ieee802-ethernet-interface]: https://github.com/kernelkit/infix/blob/f0c23ca/src/confd/yang/ieee802-ethernet-interface%402019-06-21.yang
[infix-ethernet-interface]: https://github.com/kernelkit/infix/blob/f0c23ca/src/confd/yang/infix-ethernet-interface%402024-02-27.yang
[infix-containers]: https://github.com/kernelkit/infix/blob/f0c23ca/src/confd/yang/infix-containers%402024-02-01.yang
[infix-dhcp-client]: https://github.com/kernelkit/infix/blob/f0c23ca/src/confd/yang/infix-dhcp-client%402024-01-30.yang
[infix-hardware]:  https://github.com/kernelkit/infix/blob/f0c23ca/src/confd/yang/infix-hardware%402024-01-18.yang
[infix-if-bridge]: https://github.com/kernelkit/infix/blob/f0c23ca/src/confd/yang/infix-if-bridge%402024-02-19.yang
[infix-if-type]:   https://github.com/kernelkit/infix/blob/f0c23ca/src/confd/yang/infix-if-type%402024-01-29.yang
[infix-if-veth]:   https://github.com/kernelkit/infix/blob/f0c23ca/src/confd/yang/infix-if-veth%402023-06-05.yang
[infix-if-vlan]:   https://github.com/kernelkit/infix/blob/f0c23ca/src/confd/yang/infix-if-vlan%402023-10-25.yang
[infix-ip]:        https://github.com/kernelkit/infix/tree/f0c23ca/src/confd/yang/infix-ip%402023-09-14.yang
[infix-routing]:   https://github.com/kernelkit/infix/blob/f0c23ca/src/confd/yang/infix-routing%402024-01-09.yang
[infix-services]:  https://github.com/kernelkit/infix/blob/f0c23ca/src/confd/yang/infix-services%402023-10-16.yang
[infix-system-software]: https://github.com/kernelkit/infix/tree/f0c23ca/src/confd/yang/infix-system-software%402023-06-27.yang
[ONIE]: https://opencomputeproject.github.io/onie/design-spec/hw_requirements.html

### Changes

- New hardware support: NanoPi R2S from FriendlyELEC, a simple two-port router
- Static routing support, now also for IPv6
- Dynamic routing support with OSPFv2, limited (see `infix-routing.yang` for
  deviations), but still usable in most relevant use-cases.  If you are using
  this and are interested in more features, please let us know!
  - Multiple area support, including different area types
  - Route redistribution
  - Default route injection
  - Full integration with Bidirectional Forward Detection (BFD)
  - Operational status, including but not limited to:
    * OSPF Router ID
    * Neighbor status
    * OSPF routing table
	* Interface type, incl. passive status
  - For more information, see `doc/networking.md`
- Support for disabling USB ports in `startup-config` (no auto-mount yet!)
- Initial support for Docker containers, see documentation for details:
  - Custom Infix model, see `infix-containers.yang` for details
  - Add image URL/location and volumes/mounts/interfaces to configuration,
    the system ensures the image is downloaded and container created in the
	background before launching it.  If now networking is available the job
	is queued and retried every time a new network route is learned
  - Status and actions (stop/start/restart) available in operational datastore
  - Possible to move physical switch ports inside container, see docs
  - Possible to bundle OCI archives in Infix image, as well as storing any
    file content in `factory-config` to override container image defaults
- IEEE Ethernet interface:
  - Support for setting port speed/duplex or auto-negotiating
  - New per-port counters, augments to IEEE model added in infix-ethernet.yang:
    `in-good-octets`, `out-good-octets`
- Many updates to DHCPv4 client YANG model:
  - new options, see `infix-dhcp-client.yang` for details:
    - Default options: subnet, router, dns+domain, hostname, broadcast, ntpsrv
    - Set NTP servers, require NTP client in ietf-system to be enabled, will
	  be treated as non-preferred sources, configured `prefer` servers wins
    - Learn DNS servers, statically configured servers always takes precedence
    - Install routes, not only from option 3, but also options 121 and 249
  - Support for ARP:ing for client lease (default enabled)
  - Configurable route metrics, by default metric 100 to allow static routes
    to win over DHCP routes, useful for backup DHCP connections
- IETF Hardware data: added YANG model for vital product data representation,
  and augments for initial USB support (enable/disable)
- IETF System:
  - the `motd` augment in `infix-system.yang` for *Message of the Day* has
    been marked as obsolete and replaced with `motd-banner`.  The new setting
    is of type *binary* and allows control codes and multi-line content to be
    stored.  The legacy `motd` will remain for the foreseeable future and
    takes precedence over the new `motd-banner` setting
  - new `text-editor` augment in `infix-system.yang` to select the backend for
    the new `text-editor` command: `emacs`, `nano`, or `vi`
- Many updates to the test system, *Infamy*, incl. new Quick Start Guide in
  updated `doc/testing.md` to help new developers get started
- Add `htop` to default builds, useful for observing and attaching (strace)
- Change the default shell of the `admin` user from `clish` to `bash`.  Change
  required for factory production and provisioning reasons.  Only affects the
  built-in default, customer specific `factory-config`'s are not affected!
- CLI: the `set` command on a boolean can now be used without an argument,
  `set boolean` sets the boolean option to true
- CLI: new command `change`, for use with ietf-system user passwords, starts
  an interactive password dialog, including confirmation entry.  The resulting
  password is by default salted and hashed using sha512crypt
- CLI: new command `text-editor`, for use with binary fields, e.g., `content`
  for file mounts in containers, or the new `motd-banner`:

        admin@infix-c0-ff-ee:/config/system/> text-editor motd-banner
        ... exit with Ctrl-x Ctrl-c ...
        admin@infix-c0-ff-ee:/config/system/> show
        motd-banner VGhpcyByZWxlYXNlIHdhcyBzcG9uc29yZWQgYnkgQWRkaXZhIEVsZWt0cm9uaWsK;

- CLI: new admin-exec command `show ntp [sources]`
- CLI: new admin-exec command `show dns` to display DNS client status
- CLI: new admin-exec command `show ospf [subcommand]`
- CLI: new admin-exec command `show container [subcommand]`
- CLI: new admin-exec command `show hardware` only USB port status for now
- CLI: updates to the `show interfaces` command to better list bridge VLANs

### Fixes

- Fix #177: ensure bridge is not bridge port to itself
- Fix #259: failure to `copy factory-config startup-config` in CLI
- Fix #278: allow DHCP client to set system hostname (be careful)
- Fix #283: hostname in DHCP request adds quotation marks
- Fix #294: drop stray `v` from version suffix in release artifacts
- Fix #298: drop privileges properly before launching user `shell` in CLI
- Fix #312: race condition in `ipv4_autoconf.py`, causes test to block forever
- Backport upstream fix to netopeer2-server for fetching YANG models that
  refer to submodules over NETCONF
- CLI: drop developer debug in `set` command
- Fix out-of-place `[OK]` messages at shutdown/reboot
- Fix garbled syslog messages due to unicode in Infix tagline, drop unicode


[v23.11.0][] - 2023-11-30
-------------------------

> **Note:** this is the first release where the `root` account is disabled in
> default builds.  Only the `admin` user, generated from `factory-config`, can
> log in to the system.  This can be changed only in developer builds: `make
> menuconfig` -> System configuration -> `[*]Enable root login with password`

### YANG Status

 - [ieee802-ethernet-interface][]: Currently supported (read-only) features:
   - Status of auto-negotiation, and if enabled.
   - Current speed and duplex
   - Frame counters:

    | **YANG**                    | **Linux / Ethtool**               |
    |-----------------------------|-----------------------------------|
    | `out-frames`                | `FramesTransmittedOK`             |
    | `out-multicast-frames`      | `MulticastFramesXmittedOK`        |
    | `out-broadcast-frames`      | `BroadcastFramesXmittedOK`        |
    | `in-total-octets`           | `FramesReceivedOK`                |
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

 - [ietf-system][]:
   - **augments:**
     - MotD (Message of the Day)
	 - User login shell, default: `/bin/false` (no SSH or console login)
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
   - [ietf-routing][]: Base model for routing
   - [ietf-ipv4-unicast-routing][]: Static unicast routing, incl. operational
     data, i.e., setting static IPv4 routes and reading IPv4 routing table
   - [infix-ethernet-interface][]: deviations for ieee802-ethernet-interface
   - [infix-routing][]: Limit ietf-routing to one instance `default` per
     routing protocol, also details unsupported features (deviations)
   - [infix-if-bridge][]: Linux bridge interfaces with native VLAN support
   - [infix-if-type][]: deviation for interface types, limiting number
     to supported types only.  New identities are derived from default
     IANA interface types, ensuring compatibility with other standard
     models, e.g., `ieee802-ethernet-interface.yang`
   - [infix-if-veth][]: Linux VETH pairs
   - [infix-if-vlan][]: Linux VLAN interfaces, e.g. `eth0.10`
 - **Configurable services:**
   - [ieee802-dot1ab-lldp][]: stripped down to an `enabled` setting
   - [infix-services][]: support for enabling mDNS service/device discovery

[tzdata]:          https://www.iana.org/time-zones
[ietf-system]:     https://www.rfc-editor.org/rfc/rfc7317.html
[ietf-interfaces]: https://www.rfc-editor.org/rfc/rfc7223.html
[ietf-ip]:         https://www.rfc-editor.org/rfc/rfc8344.html
[ietf-if-vlan-encapsulation]: https://www.ietf.org/id/draft-ietf-netmod-sub-intf-vlan-model-08.html
[ietf-routing]:     https://www.rfc-editor.org/rfc/rfc8349
[ietf-ipv4-unicast-routing]: https://www.rfc-editor.org/rfc/rfc8349#page-29
[ieee802-dot1ab-lldp]: https://github.com/kernelkit/infix/blob/985c2fd/src/confd/yang/ieee802-dot1ab-lldp%402022-03-15.yang
[ieee802-ethernet-interface]: https://github.com/kernelkit/infix/blob/985c2fd/src/confd/yang/ieee802-ethernet-interface%402019-06-21.yang
[infix-ethernet-interface]: https://github.com/kernelkit/infix/blob/985c2fd/src/confd/yang/infix-ethernet-interface%402023-11-22.yang
[infix-if-bridge]: https://github.com/kernelkit/infix/blob/985c2fd/src/confd/yang/infix-if-bridge%402023-11-08.yang
[infix-if-type]:   https://github.com/kernelkit/infix/blob/985c2fd/src/confd/yang/infix-if-type%402023-08-21.yang
[infix-if-veth]:   https://github.com/kernelkit/infix/blob/985c2fd/src/confd/yang/infix-if-veth%402023-06-05.yang
[infix-if-vlan]:   https://github.com/kernelkit/infix/blob/985c2fd/src/confd/yang/infix-if-vlan%402023-10-25.yang
[infix-ip]:        https://github.com/kernelkit/infix/tree/985c2fd/src/confd/yang/infix-ip%402023-09-14.yang
[infix-routing]:   https://github.com/kernelkit/infix/blob/985c2fd/src/confd/yang/infix-routing%402023-11-23.yang
[infix-services]:  https://github.com/kernelkit/infix/blob/985c2fd/src/confd/yang/infix-services%402023-10-16.yang
[infix-system-software]: https://github.com/kernelkit/infix/tree/985c2fd/src/confd/yang/infix-system-software%402023-06-27.yang

### Changes

- The CLI built-in command `password generate` has been changed to use the
  secure mode of the `pwgen` tool, and 13 chars for increased entropy
- The `qemu.sh -c` command, available in developer builds and the release zip,
  can now be used to modify the RAM size and enable VPD emulation
- Add support for overriding generated factory defaults in derivatives
  using a `/etc/confdrc.lcocal` file -- incl. updated branding docs.
- Add support for detecting factory reset condition from a bootloader
- Ensure `/var` is also cleared (properly) during factory reset
- Add support for port auto-negotiation status in operational datastore
- Add CLI support for showing veth pairs in `show interfaces`
- Speedups to CLI detailed view of a single interface
- Updated documentation of VLAN interfaces and VLAN filtering bridge
- Updated documentation for how to customize services in *Hybrid Mode*
- In RMA mode (runlevel 9), the system no longer has any login services
- Disable `root` login in all NETCONF builds, only `admin` available
- Add support for VPD data in ONIE EEPROM format
- Add `iito`, the intelligent input/output daemon for LED control
- Add port autoneg and speed/duplex status to operational data
- Upgrade Linux to v6.5.11, with kkit extensions
- Add support for static IPv4 routing using `ietf-routing@2018-03-13.yang` and
  `ietf-ipv4-unicast-routing@2018-03-13.yang`, one `default` instance only
- Add support for partitioning and self-provisioning of new devices
- Add support for reading `admin` user's default password from VPD.  Devices
  that do not have a VPD can set a password hash in the device tree
- Add support for upgrading software bundles (images) from the CLI.
  Supported remote servers: ftp, tftp, and http/https.
- Traversing the CLI configure context has been simplified by collapsing all
  YANG containers that only contain a single list element.  Example:
  `edit interfaces interface eth0` becomes `edit interface eth0`
- Add CLI support for creating configuration backups and transferring files
  to/from remote servers: tftp, ftp, http/https (download only). Issue #155
- Add `_netconf-ssh._tcp` record to mDNS-SD

### Fixes

- Fix #111: fix auto-inference of dynamic interface types (bridge, veth)
- Fix #125: improved feedback on invalid input in configure context
- Fix #198: drop bridge default PVID setting, for VLAN filtering bridge.
  All bridge ports must have explicit VLAN assignment (security)
- Fix #215: impossible to enable NTP client, regression from v23.06.0
- Fix regression in CLI `show factory-config` command
- Fix missing version in `/etc/os-release` variable `PRETTY_NAME`
- Fix failure to start `podman` in GNS3 (missing Ext4 filesystem feature)
- Fix initial terminal size probing in CLI when logging in from console port
- Fix CLI `show running-config`, use proper JSON format like other files
- Fix caching of libyang module references in confd.  Loading other plugins to
  sysrepo-plugind modifies these references, which may can cause corruption
- Fix missing `v` in `VERSION`, `VERSION_ID`, and `IMAGE_VERSION` in
  `/etc/os-release` and other generated files for release builds.


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
[UNRELEASED]: https://github.com/kernelkit/infix/compare/v24.02.0...HEAD
[v24.02.0]:   https://github.com/kernelkit/infix/compare/v23.11.0...v24.02.0
[v23.11.0]:   https://github.com/kernelkit/infix/compare/v23.10.0...v23.11.0
[v23.10.0]:   https://github.com/kernelkit/infix/compare/v23.09.0...v23.10.0
[v23.09.0]:   https://github.com/kernelkit/infix/compare/v23.08.0...v23.09.0
[v23.08.0]:   https://github.com/kernelkit/infix/compare/v23.06.0...v23.08.0
[v23.06.0]:   https://github.com/kernelkit/infix/compare/BASE...v23.06.0
[sysrepo]:    https://www.sysrepo.org/
[netopeer2]:  https://github.com/CESNET/netopeer2
