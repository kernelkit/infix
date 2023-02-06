<img align="right" src="doc/text3134.png" alt="Infix Linux Networking Made Easy">

Infix is an embedded Linux Network Operating System (NOS)
based on [Buildroot][1], [Finit][2], and [ifupdown-ng][3].

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
well as evaluation, demo and [training][] purposes, e.g. using [GNS3][].


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


[0]: https://github.com/kernelkit/infix/releases/tag/latest
[1]: https://buildroot.org/
[2]: https://github.com/troglobit/finit
[3]: https://github.com/ifupdown-ng/ifupdown-ng
[9]: https://docs.gns3.com/docs/using-gns3/beginners/import-gns3-appliance/
[QEMU]: https://www.qemu.org/
[GNS3]: https://gns3.com/
[training]: https://addiva-elektronik.github.io/
