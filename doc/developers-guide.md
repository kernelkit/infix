Developer's Guide
=================

Building
--------

Buildroot is almost stand-alone, it needs a few locally installed tools
to bootstrap itself.  For details, see the [excellent manual][manual].

Briefly, to build an Infix image; select the target and then make:

    make x86_64_defconfig
    make

Online help is available:

    make help

To see available defconfigs for supported targets, use:

    make list-defconfigs

> **Note:** build dependencies (Debian/Ubuntu): <kbd>sudo apt install make libssl-dev</kbd>


[manual]: https://buildroot.org/downloads/manual/manual.html


[7]: https://github.com/wkz/qeneth
