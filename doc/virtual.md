Virtual Environments
====================

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
> <kbd>sudo apt install virt-manager</kbd> -- dependencies ensure the relevant
> Qemu packages are pulled in as well.  This trick, installing
> [virt-manager][virt], helps set up Qemu networking on your system.

A virtualized Infix x86_64 instance can be launched from a Linux system, with
[Qemu][] installed, by issuing:

```
$ ./qemu/run.sh
...
```

from an unpacked [release tarball][rels].  From a built source tree of Infix
the same functionality is bundled as:

```
$ make run
...
```

To change settings, e.g. networking, RAM, or number of interfaces, use
<kbd>make run-menuconfig</kbd> from a built source tree.  The same
functionality is available from a release tarball as the optional `-c`
argument: <kbd>./qemu/run.sh -c</kbd> brings up a menuconfig dialog, select
`Exit` and save the changes, then start Qemu as usual.

> [!NOTE]
> The `-c` option requires the `kconfig-frontends` package, on
> Debian/Ubuntu systems: <kbd>sudo apt install kconfig-frontends</kbd>

The Infix test suite is built around Qemu and [Qeneth][qeth], see:

* [Regression Testing with Infamy](testing.md)
* [Infamy Docker Image](https://github.com/kernelkit/infix/blob/main/test/docker/README.md)

GNS3
----

The Infix appliance is available directly from the [GNS3 Marketplace][mket] --
release tarballs no longer include a `.gns3a` file.  To [install it][APPL],
open GNS3 and go to **File → New Template**, select *Install an appliance from
the GNS3 server*, and search for Infix.  When asked for a disk image, click
**Download** to fetch it directly, or point GNS3 to a `.qcow2` file downloaded
from the [releases page][rels].

Infix is in the "Router" category, it comes with 10 interfaces available by
default for use as switch ports or routing.

For a complete walk-through, from installing GNS3 to building and verifying
your first topology, see the blog post [Infix in GNS3][blog].

[Qemu]: https://www.qemu.org/
[GNS3]: https://gns3.com/
[virt]: https://virt-manager.org/
[rels]: https://github.com/kernelkit/infix/releases
[qeth]: https://github.com/wkz/qeneth
[mket]: https://gns3.com/marketplace/appliances/infix
[blog]: https://www.kernelkit.org/posts/infix-in-gns3/
[APPL]: https://docs.gns3.com/docs/using-gns3/beginners/install-from-marketplace/
