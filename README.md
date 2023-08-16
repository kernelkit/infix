![Infix - Linux <3 NETCONF](doc/logo.png)

Introduction
------------

Infix is a Linux Network Operating System (NOS) based on [Buildroot][1],
and [sysrepo][2].  A powerful mix that ease porting to different target
platforms, simplify long-term maintenance, and also provide made-easy
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


Topics
------

 - **CLI Topics**
   - [Introduction to the CLI](doc/cli/introduction.md)
   - [CLI User's Guide](doc/cli/tutorial.md)
   - [Quick Overview](doc/cli/quick.md)

 - **Infix In-Depth**
   - [Infix Variants](doc/variant.md)
   - [Boot Procedure](doc/boot.md)
   - [Containers in Infix](doc/container.md)
   - [Developer's Guide](doc/developers-guide.md)
   - [Discover Your Device](doc/discovery.md)
   - [Virtual Environments](doc/virtual.md)


Origin & Licensing
------------------

Infix is entirely built on Open Source components (packages).  Most of
them, as well as the build system with its helper scripts and tools, is
from [Buildroot][1], which is distributed under the terms of the GNU
General Public License (GPL).  See the file COPYING for details.

Some files in Buildroot contain a different license statement.  Those
files are licensed under the license contained in the file itself.

Buildroot and Infix also bundle patch files, which are applied to the
sources of the various packages.  Those patches are not covered by the
license of Buildroot or Infix.  Instead, they are covered by the license
of the software to which the patches are applied.  When said software is
available under multiple licenses, the patches are only provided under
the publicly accessible licenses.

Infix releases include the license information covering all Open Source
packages.  This is extracted automatically at build time using the tool
`make legal-info`.  Any proprietary software built on top of Infix, or
Buildroot, would need separate auditing to ensure it does not link with
any GPL[^2] licensed library.

[^1]: or RESTCONF, <https://datatracker.ietf.org/doc/html/rfc8040>, for
    mode information, see [Infix Variants](doc/variant.md).
[^2]: Infix image builds use GNU libc (GLIBC) which is covered by the
	[LGPL][8].  The LGPL *does allow* proprietary software, as long as
	said software is linking dynamically, [not statically][5], to GLIBC.

[1]: https://buildroot.org/
[2]: https://www.sysrepo.org/
[5]: https://lwn.net/Articles/117972/
[8]: https://en.wikipedia.org/wiki/GNU_Lesser_General_Public_License
