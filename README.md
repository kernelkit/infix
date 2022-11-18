Inf/IX
======

Embedded Linux distribution built on [Buildroot][Buildroot] and
[Finit][Finit].


Hardware
--------

### aarch64

By default, Inf/IX builds with support for the following boards (you
may enable additional boards in the config, of course):

- Marvell CN-9130 CRB
- Microchip SparX-5i PCB135 (eMMC)

See the aarch64 specific [documentation](board/aarch64/README.md) for more
information.

### amd64

Primarily intended to be run under QEMU.


QEMU
----

A virtualized instance can easily be launched by issuing `make run`.

Some settings, e.g. networking, can be configured via `make
menuconfig` under `External options -> QEMU virtualization`.

[Buildroot]: https://buildroot.org/
[Finit]: https://github.com/troglobit/finit
