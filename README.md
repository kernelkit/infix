```
_      __     ___    _        ______    __      __   ____   _
(_    _) |    \  |  | \    ___)     )  / (_    _) \  \  /  /
  |  |   |  |\ \ |  |  |  (__      /  /    |  |    \  \/  /
  |  |   |  | \ \|  |  |   __)    /  /     |  |     >    <
 _|  |_  |  |  \    |  |  (      /  /     _|  |_   /  /\  \
(      )_|  |___\   |_/    \____/  (_____(      )_/  /__\  \_
```
An embedded Linux Network Operating System (NOS) based on [Buildroot][1] and [Finit][2].


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

Primarily intended to be run under QEMU for development & test as well
as evaluation purposed.


QEMU
----

A virtualized instance can easily be launched by issuing `make run`.

Some settings, e.g. networking, can be configured via `make
menuconfig` under `External options -> QEMU virtualization`.

[1]: https://buildroot.org/
[2]: https://github.com/troglobit/finit
