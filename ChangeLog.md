Change Log
==========

All notable changes to the project are documented in this file.


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
[UNRELEASED]: https://github.com/kernelkit/infix/compare/v23.06.0...HEAD
[v23.06.0]:   https://github.com/kernelkit/infix/compare/BASE...v23.06.0
[sysrepo]:    https://www.sysrepo.org/
[netopeer2]:  https://github.com/CESNET/netopeer2
