Package Override
================

This guide demonstrates how the `local.mk` file is utilized to override
a Linux Buildroot package.  As an example we use `tcpdump` to illustrate
this process.

> For a comprehensive guide to utilizing Buildroot during development,
> including the `<pkg>_OVERRIDE_SRCDIR` mechanism, shown below, please
> see [Using Buildroot during development][1] in the official docs.


Setup
-----

Since the `output/` directory is often wiped for rebuilds, make sure you
keep the file `local.mk` in the top directory and only symlink it to your
build directory:

    ~$ cd infix/
    ~/infix(main)$ touch local.mk
    ~/infix/output(main)$ ln -s ../local.mk .
    ~/infix/output(main)$ cd ..


Override
--------

Now edit the file:

    ~/infix(main)$ editor local.mk

Add an override for `tcpdump`.  The file is a Makefile snippet so you
can add a lot of things.  Comment out lines with the UNIX comment `#`
character if needed:

```
TCPDUMP_OVERRIDE_SRCDIR = /path/to/tcpdump/repo
```

Building
--------

The execution of `make tcpdump-rebuild all` triggers a process where
Buildroot synchronizes the tcpdump source code from the specified
override directory to `output/build/tcpdump-custom`, followed by the
rebuilding of the entire project.

```
~/infix$(main)$ make tcpdump-rebuild all
```

Buildroot follows a process of downloading and processing tarballs:
extraction, configuration, compilation, and installation.  The source
for each package is extracted to a temporary build directory:

    output/build/<package>-<version>    # e.g., tcpdump-4.99.4

Let's have a look at what we got:

```
~/infix$(main)$ ll /output/build/ | grep tcpdump
drwxr-xr-x   7 group user 20480 Nov 10 18:26 tcpdump-4.99.4/
drwxr-xr-x   7 group user 12288 Nov 10 18:28 tcpdump-custom/
```

As long as your local override is in place, Buildroot will use your
custom version.

> **Remember:** the build directory is ephemeral, so be careful to
> change any of the files therein.  It can be useful though during
> debugging, but just make sure to learn the difference between the
> various Buildroot commands to build, clean, reconfigure, etc.

[1]: https://buildroot.org/downloads/manual/manual.html#_using_buildroot_during_development
