Developer's Guide
=================

Cloning
-------

Please see the [Contributing](#contributing) section, below, for details
on how to fork and clone when contributing to Infix.

```bash
$ mkdir ~/Projects; cd ~/Projects
$ git clone https://github.com/kernelkit/infix.git
$ cd infix/
$ git submodule update --init
```


Building
--------

Buildroot is almost stand-alone, it needs a few locally installed tools
to bootstrap itself.  For details, see the [excellent manual][manual].

> **Note:** installation for Debian/Ubuntu based systems: <kbd>sudo apt
> install make libssl-dev</kbd>

Briefly, to build an Infix image; select the target and then make:

    make x86_64_defconfig
    make

Online help is available:

    make help

To see available defconfigs for supported targets, use:

    make list-defconfigs


Testing
-------

Manual testing can be done using Qemu by calling <kbd>make run</kbd>,
see also [Infix in Virtual Environments](virtual.md).

The Infix automated test suite is built around Qemu and [Qeneth][2], see:

 * [Testing](doc/testing.md)
 * [Docker Image](test/docker/README.md)


Contributing
------------

Infix is built from many parts, when contributing you need to set up
your own fork, create a local branch for your change, push to your fork,
and then use GitHub to create a *Pull Reqeuest*.

For this to work as painlessly as possible:

  1. Fork Infix to your own user
  2. Fork all the Infix submodules, e.g., `buildroot` to your own user
  3. Clone your fork of Infix to your laptop/workstation

```bash
$ cd ~/Projects
$ git clone https://github.com/YOUR_USER_NAME/infix.git
$ cd infix/
$ git submodule update --init
```

> **Note:** when updating/synchronizing with upstream Infix changes you
> may have to synchronize your forks as well.  GitHub have a `Sync fork`
> button in the GUI for your fork for this purpose.

[1]: https://buildroot.org/downloads/manual/manual.html
[2]: https://github.com/wkz/qeneth
