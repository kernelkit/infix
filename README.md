<img align="right" src="doc/logo.png" alt="Infix - Linux <3 NETCONF" width=480>
<details><summary><b>Documentation</b></summary>

 - **Infix In-Depth**
   - [Infix Variants](doc/variant.md)
   - [Boot Procedure](doc/boot.md)
   - [Containers in Infix](doc/container.md)
   - [Developer's Guide](doc/developers-guide.md)
   - [Discover Your Device](doc/discovery.md)
   - [Virtual Environments](doc/virtual.md)
   - [Origin & Licensing](doc/license.md)
- **CLI Topics**
   - [Introduction to the CLI](doc/cli/introduction.md)
   - [CLI User's Guide](doc/cli/tutorial.md)
   - [Quick Overview](doc/cli/quick.md)

</details>

Infix is a Linux Network Operating System (NOS) based on [Buildroot][1],
and [sysrepo][2].  A powerful mix that ease porting to different
platforms, simplify long-term maintenance, and provide made-easy
management using NETCONF[^1] (remote) or the built-in [CLI][3].

<details><summary><b>Example CLI Session</b></summary>

The CLI configure context is automatically generated from the loaded
YANG models and their corresponding [sysrepo][2] plugins.  The following
is brief example of how to set the IP address of an interface:

```
admin@infix-12-34-56:/> configure
admin@infix-12-34-56:/config/> edit interfaces interface eth0
admin@infix-12-34-56:/config/interfaces/interface/eth0/> set ipv4 <TAB>
      address     autoconf bind-ni-name      enabled
	  forwarding  mtu      neighbor
admin@infix-12-34-56:/config/interfaces/interface/eth0/> set ipv4 address 192.168.2.200 prefix-length 24
admin@infix-12-34-56:/config/interfaces/interface/eth0/> show
type ethernetCsmacd;
ipv4 address 192.168.2.200 prefix-length 24;
ipv6 enabled true;
admin@infix-12-34-56:/config/interfaces/interface/eth0/> diff
interfaces {
  interface eth0 {
+    ipv4 {
+      address 192.168.2.200 {
+        prefix-length 24;
+      }
+    }
  }
}
admin@infix-12-34-56:/config/interfaces/interface/eth0/> leave
admin@infix-12-34-56:/> show interfaces brief
lo               UNKNOWN        00:00:00:00:00:00 <LOOPBACK,UP,LOWER_UP>
eth0             UP             52:54:00:12:34:56 <BROADCAST,MULTICAST,UP,LOWER_UP>
admin@infix-12-34-56:/> show ip brief
lo               UNKNOWN        127.0.0.1/8 ::1/128
eth0             UP             192.168.2.200/24 fe80::5054:ff:fe12:3456/64
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

An x86_64 build is also available, primarily intended for development
and testing, but can also be used for evaluation and demo purposes.  For
more information, see: [Infix in Virtual Environments](doc/virtual.md).

> See the [GitHub Releases](https://github.com/kernelkit/infix/releases)
> page for our pre-built images.  The *Latest Build* has the bleeding
> edge images, if possible we recommend using a versioned release.
>
> For *customer specific builds* of Infix, see your product repository.

[^1]: NETCONF or RESTCONF, <https://datatracker.ietf.org/doc/html/rfc8040>,
    for more information, see [Infix Variants](doc/variant.md).

[1]: https://buildroot.org/
[2]: https://www.sysrepo.org/
[3]: doc/cli/introduction.md
