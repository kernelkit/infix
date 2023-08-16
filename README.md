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
management using NETCONF[^1].

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

[^1]: or RESTCONF, <https://datatracker.ietf.org/doc/html/rfc8040>, for
    mode information, see [Infix Variants](doc/variant.md).

[1]: https://buildroot.org/
[2]: https://www.sysrepo.org/
