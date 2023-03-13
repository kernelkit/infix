<img align="right" src="doc/text3134.png" alt="Infix Linux Networking Made Easy">

Infix is an embedded Linux Network Operating System (NOS) based on
[Buildroot][1], [Finit][2], [ifupdown-ng][3], and [Clixon][6].
Providing an easy-to-maintain and easy-to-port Open Source base for
networked equipment.

See the [GitHub Releases](https://github.com/kernelkit/infix/releases)
page for out pre-built images.  The *Latest Build* has the bleeding edge
images, if possible we recommend using a versioned release.

> Login with user 'root', no password by default on plain builds.  See
> the online `help` command for an introduction to the system.


Hardware
--------

### aarch64

By default, Infix builds with support for the following boards (you
may enable additional boards in the config, of course):

- Marvell CN9130 CRB
- Marvell EspressoBIN
- Microchip SparX-5i PCB135 (eMMC)

See the aarch64 specific [documentation](board/aarch64/README.md) for more
information.

### amd64

Primarily intended to be run under [QEMU][] for development & test as
well as evaluation, demo and [training][] purposes, e.g. using [GNS3][]
or [Qeneth][7].


QEMU
----

A virtualized instance can easily be launched from a Linux system, with
Qemu installed, by issuing `make run`.

Some settings, e.g. networking, can be configured via `make menuconfig`
under `External options -> QEMU virtualization`.


GNS3
----

Download the [latest build][0] of amd64, unpack in a dedicated directory
and use ["Import Appliance"][9] to install the `.gns3a` file into GNS3.
Infix will show up in the "Router" category, it has 10 interfaces
available by default for use as switch ports or routing.


Origin & Licensing
------------------

Infix is entirely built on Open Source components (packages).  Most of
them, as well as the build system with its helper scripts and tools, is
from [Buiildroot][1], which is distributed under the terms of the GNU
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
any GPL[^1] licensed library.

[^1]: Infix image builds use GNU libc (GLIBC) which is covered by the
	[LGPL][4].  The LGPL *does allow* proprietary software, as long as
	said software is linking dynamically, [not statically][5], to GLIBC.

[0]: https://github.com/kernelkit/infix/releases/tag/latest
[1]: https://buildroot.org/
[2]: https://github.com/troglobit/finit
[3]: https://github.com/ifupdown-ng/ifupdown-ng
[4]: https://en.wikipedia.org/wiki/GNU_Lesser_General_Public_License
[5]: https://lwn.net/Articles/117972/
[6]: https://github.com/clicon/clixon
[7]: https://github.com/wkz/qeneth
[9]: https://docs.gns3.com/docs/using-gns3/beginners/import-gns3-appliance/
[QEMU]: https://www.qemu.org/
[GNS3]: https://gns3.com/
[training]: https://addiva-elektronik.github.io/
