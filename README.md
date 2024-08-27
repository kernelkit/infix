[![License Badge][]][License] [![GitHub Status][]][GitHub] [![Coverity Status][]][Coverity Scan] [![Discord][discord-badge]][discord-url]

<img align="right" src="doc/logo.png" alt="Infix - Linux <3 NETCONF" width=480 border=10>

Infix is a free, Linux based, immutable Network Operating System (NOS)
built on [Buildroot][1], and [sysrepo][2].  A powerful mix that ease
porting to different platforms, simplify long-term maintenance, and
provide made-easy management using NETCONF, RESTCONF, or the built-in
command line interface (CLI) from a console or SSH login.

> Click the **▶ Example CLI Session** foldout below for an example, or
> head on over to the [Infix Documentation](doc/README.md) for more
> information on how to set up the system.

Although primarily focused on switches and routers, the core values
may be appealing for other use-cases as well:

- Runs from a squashfs image on a read-only partition
- Single configuration file on a separate partition
- Built around YANG with standard IETF models
- Linux switchdev provides open switch APIs
- Atomic upgrades to secondary partition
- Highly security focused

An immutable[^1] operating system enhances security and inherently makes
it maintenance-free.  Configuration and data, e.g, containers, is stored
on separate partitions to ensure complete separation from system files
and allow for seamless backup, restore, and provisioning.

In itself Infix is perfectly suited for dedicated networking tasks and
native support for Docker containers provides a versatile platform that
can easily be adapted to any customer need.  Be it legacy applications,
network protocols, process monitoring, or edge data analysis, it can run
close to end equipment.  Either directly connected on dedicated Ethernet
ports or indirectly using virtual network cables to exist on the same
LAN as other connected equipment.

The simple design of Infix provides complete control over both system
and data, minimal cognitive burden, and makes it incredibly easy to get
started.

<details><summary><b>Example CLI Session</b></summary>

The CLI configure context is automatically generated from the loaded
YANG models and their corresponding [sysrepo][2] plugins.  The following
is brief example of how to set the IP address of an interface:

```
admin@infix-12-34-56:/> configure
admin@infix-12-34-56:/config/> edit interface eth0
admin@infix-12-34-56:/config/interface/eth0/> set ipv4 <TAB>
      address     autoconf bind-ni-name      enabled
	  forwarding  mtu      neighbor
admin@infix-12-34-56:/config/interface/eth0/> set ipv4 address 192.168.2.200 prefix-length 24
admin@infix-12-34-56:/config/interface/eth0/> show
type ethernet;
ipv4 {
  address 192.168.2.200 {
    prefix-length 24;
  }
}
ipv6
admin@infix-12-34-56:/config/interface/eth0/> diff
interfaces {
  interface eth0 {
+    ipv4 {
+      address 192.168.2.200 {
+        prefix-length 24;
+      }
+    }
  }
}
admin@infix-12-34-56:/config/interface/eth0/> leave
admin@infix-12-34-56:/> show interfaces
INTERFACE       PROTOCOL   STATE       DATA
eth0            ethernet   UP          52:54:00:12:34:56
                ipv4                   192.168.2.200/24 (static)
                ipv6                   fe80::5054:ff:fe12:3456/64 (link-layer)
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
admin@infix-12-34-56:/> copy running-config startup-config
```

[Click here][3] for more details.
</details>

Infix can run on many different types of architectures and boards, much
thanks to Linux and Buildroot.  Currently the focus is on 64-bit ARM
devices, optionally with switching fabric supported by Linux switchdev.
The [following boards](board/aarch64/README.md) are fully supported:

 - Marvell CN9130 CRB
 - Marvell EspressoBIN
 - Microchip SparX-5i PCB135 (eMMC)
 - StarFive VisionFive2
 - NanoPi R2S

An x86_64 build is also available, primarily intended for development
and testing, but can also be used for evaluation and demo purposes.  For
more information, see: [Infix in Virtual Environments](doc/virtual.md).

> See the [GitHub Releases](https://github.com/kernelkit/infix/releases)
> page for our pre-built images.  The *[Latest Build][]* has bleeding
> edge images, if possible we recommend using a versioned release.
>
> For *customer specific builds* of Infix, see your product repository.

[^1]: An immutable operating system is one with read-only file systems,
    atomic updates, rollbacks, declarative configuration, and workload
    isolation.  All to improve reliability, scalability, and security.
    For more information, see <https://ceur-ws.org/Vol-3386/paper9.pdf>
    and <https://www.zdnet.com/article/what-is-immutable-linux-heres-why-youd-run-an-immutable-linux-distro/>.

[1]: https://buildroot.org/
[2]: https://www.sysrepo.org/
[3]: doc/cli/introduction.md
[Latest Build]:    https://github.com/kernelkit/infix/releases/tag/latest
[License]:         https://en.wikipedia.org/wiki/GPL_license
[License Badge]:   https://img.shields.io/badge/License-GPL%20v2-blue.svg
[GitHub]:          https://github.com/kernelkit/infix/actions/workflows/build.yml/
[GitHub Status]:   https://github.com/kernelkit/infix/actions/workflows/build.yml/badge.svg
[Coverity Scan]:   https://scan.coverity.com/projects/29393
[Coverity Status]: https://scan.coverity.com/projects/29393/badge.svg
[discord-badge]:   https://img.shields.io/discord/1182652155618918411.svg?logo=discord
[discord-url]:     https://discord.gg/6bHJWQNVxN
