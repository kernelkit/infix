<img align="right" src="doc/text3134.png" alt="Infix Linux Networking Made Easy">

* [Introduction](#introduction)
* [Hardware](#hardware)
* [Qemu](#qemu)
* [GNS3](#gns3)
* [Building](#building)
* [Origin & Licensing](origin--licensing)


Introduction
------------

Infix is an embedded Linux Network Operating System (NOS) based on
[Buildroot][1], [Finit][2], [ifupdown-ng][3], and [sysrepo][6].
Providing an easy-to-maintain and easy-to-port Open Source base for
networked equipment.

> See the [GitHub Releases](https://github.com/kernelkit/infix/releases)
> page for out pre-built images.  The *Latest Build* has the bleeding edge
> images, if possible we recommend using a versioned release.

Infix has two main *flavors*, or defconfigs:

 - **NETCONF:** the default, is configured using the `cli` tool, or any
   external NETCONF client, after boot.  It uses sysrepo as the data
   store to generate configuration files in `/etc` and control the
   system daemons, e.g., enable DHCP client on an interface.

 - **Classic:** built from `$ARCH_classic_defconfig`.  Here it is up to
   the administrator to modify configuration files in `/etc` and control
   the system daemons using the `initctl` tool.  After login, see the
   online `help` command for an introduction to the system.

Both flavors have a `root` user, which is only allowed to log in from
the console port, no password by default on standard builds.  There is
also a `factory` user, password `reset`, to perform factory reset on
systems that do not have a reset button.

Additionally, the standard builds also have an `admin` user, which is
allowed to log in from remote, password `admin` on standard builds.  It
is the recommended account to use for managing Infix.


Hybrid Mode
-----------

Since Infix is under heavy development, it does not yet have all its
bells and whistles in place, in particular the default build.  To that
end it is possible to manually manage certain services that are not yet
possible to configure using NETCONF.

At bootstrap Finit can optionally start scripts from a [run-parts(8)][]
like directory: `/cfg/start.d`.  To enable this mode, see the following
example:

```sh
root@infix:~$ mkdir /cfg/start.d
root@infix:~$ cd /cfg/start.d
root@infix:/cfg/start.d$ cat <<EOF >10-enable-ospf.sh
#!/bin/sh
# Use vtysh to modify the OSPF configuration
rm /etc/frr/frr.conf
ln -s /cfg/frr/frr.conf /etc/frr/
initctl enable zebra
initctl enable ospfd
initctl enable bfdd
EOF
root@infix:/cfg/start.d$ chmod +x 10-enable-ospf.sh
```

Reboot to activate the changes.  To activate the changes without
rebooting, run the script and call `initctl reload`.


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

### x86_64

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

Download the [latest build][0] of the `x86_64`, or `x86_64_classic`
flavor.  Unpack in a dedicated directory and use ["Import Appliance"][9]
to install the `.gns3a` file into GNS3.  Infix (`x86_64`) is in the
"Router" category, it has 10 interfaces available by default for use as
switch ports or routing.  The *classic* build only has one interface by
default, geared more towards acting as an end device.


Building
--------

Buildroot is almost stand-alone, but need a few locally installed tools
to bootstrap itself.  For details, see the [excellent manual][manual].

Briefly, to build an Infix image; select the target and then make:

    make x86_64_defconfig
    make

Online help is available:

    make help

To see available defconfigs for supported targets, use:

    make list-defconfigs

> **Note:** build dependencies (Debian/Ubuntu): <kbd>sudo apt install make libssl-dev</kbd>


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
[6]: https://www.sysrepo.org/
[7]: https://github.com/wkz/qeneth
[9]: https://docs.gns3.com/docs/using-gns3/beginners/import-gns3-appliance/
[QEMU]: https://www.qemu.org/
[GNS3]: https://gns3.com/
[training]: https://addiva-elektronik.github.io/
[manual]: https://buildroot.org/downloads/manual/manual.html
[run-parts(8)]: https://manpages.ubuntu.com/manpages/trusty/man8/run-parts.8.html
