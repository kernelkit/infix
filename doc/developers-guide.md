Developer's Guide
=================

Please note, by default the `root` account is disabled in Infix NETCONF
builds.  Meaning, the only way to access the system is with the `admin`
account, which is created based on credentials found in the VPD area --
for Qemu devices this is emulated using `qemu_fw_cfg`.

For developers this can be quite frustrating to be blocked from logging
in to debug the system.  So we recommend enabling the `root` account in
the Buildroot `make menuconfig` system.

    make menuconfig
         -> System configuration
            -> [*]Enable root login with password


Cloning
-------

When [pre-built releases][0] are not enough, for instance when you want
to add or modify some Open Source components, you can clone the Infix
tree to your PC:

```bash
$ mkdir ~/Projects; cd ~/Projects
$ git clone https://github.com/kernelkit/infix.git
$ cd infix/
$ git submodule update --init
```

> Please see the [Contributing](#contributing) section, below, for
> details on how to fork and clone when contributing to Infix.


### Customer Builds

Customer builds add product specific device trees, more OSS packages,
e.g., Frr and podman, and sometimes integrates proprietary software.
What's *important to remember*, however, is that they are all made by
setting up Infix as a GIT submodule, similar to how Infix set up a GIT
submodule for Buildroot.

So, in addition to using the customer's specific defconfig(s), one must
also make sure to update *all submodules*, otherwise you will likely end
up with a broken build.

```bash
$ ...
$ git submodule update --init --recursive
                              ~~~~~~~~~~~
```

Other caveats should be documented in the customer specific trees.


Building
--------

Buildroot is almost stand-alone, it needs a few locally installed tools
to bootstrap itself.  The most common ones are usually part of the base
install of the OS, but specific ones for building need the following.
The instructions here are for Debian/Ubuntu based systems (YMMV):

```bash
$ sudo apt install bc binutils build-essential bzip2 cpio \
                   diffutils file findutils git gzip      \
                   libncurses-dev libssl-dev perl patch   \
                   python3 rsync sed tar unzip wget       \
                   autopoint bison flex
```

For testing, a few more tools and services are required on your system:

```bash
$ sudo apt install jq graphviz qemu-system-x86 qemu-system-arm \
				   ethtool gdb-multiarch tcpdump tshark
```

> For details, see the Getting Started and System Requirements sections
> of the [excellent manual][1].

To build an Infix image; select the target and then make:

    make x86_64_defconfig
    make

Online help is available:

    make help

To see available defconfigs for supported targets, use:

    make list-defconfigs


Development
-----------

Developing with Infix is the same as [developing with Buildroot][4].
When working with a package, be it locally kept sources, or when using
[`local.mk`](override-package.md), you only want to rebuild the parts
you have modified:

    make foo-rebuild

or

    make foo-reconfigure

or, as a last resort when nothing seems to bite:

    make foo-dirclean foo-rebuild

As shown here, you can combine multiple build targets and steps in one
go, like this:

    make foo-rebuild bar-rebuild all run

This rebuilds (and installs) `foo` and `bar`, the `all` target calls
on Buildroot to finalize the target filesystem and generate the images.
The final `run` argument is explained below.

### `confd`

The Infix `src/confd/` is the engine of the system.  Currently it is a
plugin for `systemd-plugind` and contains XPath subscriptions to all the
supported YANG models.

There are essentially two ways of adding support for a new YANG model:

 - The [sysrepo way][3], or
 - The Infix way, using libsrx (the `lydx_*()` functions)

The former is well documented in sysrepo, and the latter is best taught
by example, e.g., `src/confd/src/infix-dhcp.c`.  Essentially libsrx is a
way of traversing the libyang tree instead of fetching changes by XPath.

When working with `confd` you likely want to enable full debug mode,
this is how you do it:

 1. Open the file `package/confd/confd.conf`
 2. Uncomment the first line `set DEBUG=1`
 3. Change the following line to add `-v3` at the end

        [S12345] sysrepo-plugind -f -p /run/confd.pid -n -- Configuration daemon

to:

    [S12345] sysrepo-plugind -f -p /run/confd.pid -n -v3 -- Configuration daemon

Now you can rebuild `confd`, just as described above, and restart Infix:

    make confd-rebuild all run


Testing
-------

Manual testing can be done using Qemu by calling <kbd>make run</kbd>,
see also [Infix in Virtual Environments](virtual.md).

The Infix automated test suite is built around Qemu and [Qeneth][2], see:

 * [Testing](testing.md)
 * [Docker Image](../test/docker/README.md)


Contributing
------------

Infix is built from many parts, when contributing you need to set up
your own fork, create a local branch for your change, push to your fork,
and then use GitHub to create a *Pull Reqeuest*.

For this to work as painlessly as possible:

  1. Fork Infix to your own user or organization[^1]
  2. Fork all the Infix submodules, e.g., `kernelkit/buildroot` to your
     own user or organization as well
  3. Clone your fork of Infix to your laptop/workstation

If you use a GitHub organization you get the added benefit of having
local peer reviews of changes before making a pull request to the
upstream Infix repository.

```bash
$ cd ~/Projects
$ git clone https://github.com/YOUR_USER_NAME/infix.git
$ cd infix/
$ git submodule update --init
```

> **Note:** when updating/synchronizing with upstream Infix changes you
> may have to synchronize your forks as well.  GitHub have a `Sync fork`
> button in the GUI for your fork for this purpose.

[^1]: Organizations should make sure to lock the `main` (or `master`)
    branch of their clones to ensure members do not accidentally merge
    changes there.  Keeping these branches in sync with upstream Infix
    is highly recommended as a baseline and reference.  For integration
	of local changes another company-specific branch can be used instead.

[0]: https://github.com/kernelkit/infix/releases
[1]: https://buildroot.org/downloads/manual/manual.html
[2]: https://github.com/wkz/qeneth
[3]: https://netopeer.liberouter.org/doc/sysrepo/master/html/dev_guide.html
[4]: https://buildroot.org/downloads/manual/manual.html#_developer_guide
