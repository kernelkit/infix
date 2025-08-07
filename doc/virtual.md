Virtual Environments
=====================

Infix primarily targets real hardware, deployment to the cloud is not a
priority at the moment.  However, for development and testing purposes
there is an `x86_64` build that runs in [Qemu][].

These images also work with the Graphical Network Simulator ([GNS3][]),
which is a very user-friendly tool for playing around with simple to
complex network setups, verifying interoperability between vendors, etc.

QEMU
----

> [!TIP]
> Installation for Debian/Ubuntu based systems can be done by "simply":
> <kbd>sudo apt install virt-manager</kbd> -- dependencies ensure the
> relevant Qemu packages are pulled in as well.  This trick, installing
> [virt-manager][virt], helps set up Qemu networking on your system.

A virtualized Infix x86_64 instance can easily be launched from a Linux
system, with [Qemu][] installed, by issuing:

```
$ ./qemu.sh
...
```

from an unpacked [release tarball][rels].  From a built source tree of
Infix the same functionality is bundled as:

```
$ make run
...
```

To change settings, e.g. networking, <kbd>make run-menuconfig</kbd>, or
from a pre-built Infix release tarball, using <kbd>./qemu.sh -c</kbd>

The Infix test suite is built around Qemu and [Qeneth][qeth], see:

* [Regression Testing with Infamy](testing.md)
* [Infamy Docker Image](https://github.com/kernelkit/infix/blob/main/test/docker/README.md)

GNS3
----

Download the [latest build][rels] of the `x86_64`, or `aarch64` if your
host machine is Arm.  Unpack the tarball in a dedicated directory and
use ["Import Appliance"][APPL] to install the `.gns3a` file into
[GNS3][].

Infix is in the "Router" category, it comes with 10 interfaces available
by default for use as switch ports or routing.

[Qemu]: https://www.qemu.org/
[GNS3]: https://gns3.com/
[virt]: https://virt-manager.org/
[rels]: https://github.com/kernelkit/infix/releases
[qeth]: https://github.com/wkz/qeneth
[APPL]: https://docs.gns3.com/docs/using-gns3/beginners/import-gns3-appliance/
