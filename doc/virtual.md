Infix in Virtual Environments
=============================

Infix primarily targets real hardware, deployment to the cloud is not a
priority at the moment.  However, for development and testing purposes
there is an `x86_64` build that runs in [Qemu][].

These images also work with the Graphical Network Simulator ([GNS3][]),
which is a very user-friendly tool for playing around with simple to
complex network setups, verifying interoperability between vendors, etc.


QEMU
----

> **Note:** installation for Debian/Ubuntu based systems: <kbd>sudo apt
> install virt-manager</kbd> -- dependencies ensure the relevant Qemu
> packages are pulled in as well.  Installing [virt-manager][virt] helps
> set up Qemu networking on your system.

A virtualized Infix x86_64 instance can easily be launched from a Linux
system, with [Qemu][] installed, by issuing:

    ./qemu.sh

from an unpacked [release tarball][rels].  From a built source tree of
Infix the same functionality is bundled as:

    make run

To change settings, e.g. networking, <kbd>make run-menuconfig</kbd>, or
from a pre-built Infix release tarball, using <kbd>./qemu.sh -c</kbd>

The Infix test suite is built around Qemu and [Qeneth][qeth], see:

 * [Testing](testing.md)
 * [Docker Image](../test/docker/README.md)


GNS3
----

Download the [latest build][rels] of the `x86_64`, or `x86_64_classic`
flavor.  Unpack the tarball in a dedicated directory and use ["Import
Appliance"][APPL] to install the `.gns3a` file into [GNS3][].

Infix (`x86_64`) is in the "Router" category, it has with 10 interfaces
available by default for use as switch ports or routing.  The *classic*
build only has one interface by default, geared more towards acting as
an end device.

[Qemu]: https://www.qemu.org/
[GNS3]: https://gns3.com/
[virt]: https://virt-manager.org/
[rels]: https://github.com/kernelkit/infix/releases
[qeth]: https://github.com/wkz/qeneth
[APPL]: https://docs.gns3.com/docs/using-gns3/beginners/import-gns3-appliance/
