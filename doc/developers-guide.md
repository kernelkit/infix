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

> [!IMPORTANT]
> Please see the [Contributing](#contributing) section, below, for
> details on how to fork and clone when contributing to Infix.

Cloning
-------

When [pre-built releases][0] are not enough, for instance when you want
to add or modify some Open Source components, you can clone the Infix
tree to your PC:

```bash
$ mkdir ~/Projects; cd ~/Projects
$ git clone https://github.com/kernelkit/infix.git
..
$ cd infix/
$ git submodule update --init
..
```

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

> [!TIP]
> For more details, see the Getting Started and System Requirements
> sections of the [excellent Buildroot manual][1].

Buildroot is almost stand-alone, it needs a few locally installed tools
to bootstrap itself.  The most common ones are usually part of the base
install of the OS, but specific ones for building need the following.
The instructions here are for Debian/Ubuntu based systems (YMMV):

```bash
$ sudo apt install bc binutils build-essential bzip2 cpio \
                   diffutils file findutils git gzip      \
                   libncurses-dev libssl-dev perl patch   \
                   python3 rsync sed tar unzip wget       \
                   autopoint bison flex autoconf automake \
                   mtools
```

To build an Infix image; select the target and then make:

    make x86_64_defconfig
    make

Online help is available:

    make help

To see available defconfigs for supported targets, use:

    make list-defconfigs


### Test

Working with the regression test framework, *Infamy*, a few more tools
and services are required on your system:

```bash
$ sudo apt install jq graphviz qemu-system-x86 qemu-system-arm \
				   ethtool gdb-multiarch tcpdump tshark
..
```

To be able to build the test specification you also need:

```bash
$ sudo apt-get install python3-graphviz ruby-asciidoctor-pdf
..
```


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


### `statd`

The Infix status daemon, `src/statd`, is responsible for populating the
sysrepo `operational` datastore. Like `confd`, it uses XPath subscriptions,
but unlike `confd`, it relies entirely on `yanger`, a Python script that
gathers data from local linux services and feeds it into sysrepo.

To apply changes, rebuild the image:

    make python-statd-rebuild statd-rebuild all

Rebuilding the image and testing on target for every change during
development process can be tedious. Instead, `yanger` allows remote
execution, running the script directly on the host system (test
container):

    infamy0:test # ../src/statd/python/yanger/yanger -x "../utils/ixll -A ssh d3a" ieee802-dot1ab-lldp

`ixll` is a utility script that lets you run network commands using an
**interface name** instead of a hostname. It makes operations like
`ssh`, `scp`, and network discovery easier.

Normally, `yanger` runs commands **locally** to retrieve data
(e.g., `lldpcli` when handling `ieee802-dot1ab-lldp`). However, when
executed with `-x "../utils/ixll -A ssh d3a"` it redirects these
commands to a remote system connected to the local `d3a` interface via
SSH. This setup is used for running `yanger` in an
[interactive test environment](testing.md#interactive-usage). The yanger
script runs on the `host` system, but key commands are executed on the
`target` system.

For debugging or testing, you can capture system command output and
replay it later without needing a live system.

To capture:

    infamy0:test # ../src/statd/python/yanger/yanger -c /tmp/capture ieee802-dot1ab-lldp

To replay:

    infamy0:test # ../src/statd/python/yanger/yanger -r /tmp/capture ieee802-dot1ab-lldp

This is especially useful when working in isolated environments or debugging
issues without direct access to the DUT.

### Upgrading Packages

#### Buildroot

Kernelkit maintains an internal [fork of
Buildroot](https://github.com/kernelkit/buildroot), with branches
following the naming scheme `YYYY.MM.patch-kkit`
e.g. `2025.02.1-kkit`, which means a new branch should be created
whenever Buildroot is updated. These branches should contain **only**
changes to existing packages (but no new patches), modifications to
Buildroot itself or upstream backports.

KernelKit track the latest Buildroot LTS (Long-Term Support) release
and updates. The upgrade of LTS minor releases is expected to have low
impact and should be done as soon there is a patch release of
Buildroot LTS is available.

> **Depending on your setup, follow the appropriate steps below.**

🔁 If you **already have** the Buildroot repo locally

1. Navigate to the Buildroot directory
   ```bash
    $ cd buildroot
   ```
2. Pull the latest changes from KernelKit
   ```bash
   $ git pull
   ```
3. Fetch the latest tags from upstream
   ```bash
   $ git fetch upstream --tags
   ```


🆕 If you don't have the repo locally

1. Clone the Kernelkit Buildroot repository
   ```bash
   $ git clone git@github.com:kernelkit/buildroot.git
   ```

2. Add the upstream remote
   ```bash
   $ git remote add upstream https://gitlab.com/buildroot.org/buildroot.git
   ```
3. Checkout old KernelKit branch
   ```bash
   $ git checkout 2025.02.1-kkit
   ```


🛠  Continue from here (applies to both cases):

4. Create a new branch based on the **previous** KernelKit Buildroot
   release (e.g.  `2025.02.1-kkit`) and name it according to the naming scheme (e.g. `2025.02.2-kkit`)
   ```bash
   $ git checkout -b 2025.02.2-kkit
   ```
5. Rebase the new branch onto the corresponding upstream release
   ```bash
   $ git rebase 2025.02.2
   ```
> [!NOTE] It is **not** allowed to rebase the branch when bumped in Infix.

6. Push the new branch and tags
   ```bash
   $ git push origin 2025.02.2-kkit --tags
   ```
7. In Infix, checkout new branch of Buildroot
   ```bash
   $ cd buildroot
   $ git fetch
   $ git checkout 2025.02.2-kkit
   ```
8. Push changes
Commit and push the changes. Don’t forget to update the changelog.

9. Create a pull request.

> [!NOTE] Remember to set the pull request label to `ci:main` to ensure full CI coverage.


#### Linux kernel

KernelKit maintains an internal [fork of Linux
kernel](https://github.com/kernelkit/linux), with branches following
the naming scheme `kkit-linux-[version].y`, e.g. `kkit-6.12.y`, which
means a new branch should be created whenever the major kernel version
is updated. This branch should contain *all* kernel patches used by
Infix.

KernelKit track the latest Linux kernel LTS (Long-Term Support)
release and updates. The upgrade of LTS minor releases is expected to
have low impact and should be done as soon as a patch release of the
LTS Linux kernel is available.


🔁 If you **already have** the Linux kernel repo locally

1. Navigate to the Linux kernel directory
   ```bash
   $ cd linux
   ```
2. Get latest changes from KernelKit
   ```bash
   $ git pull
   ```
3. Fetch the latest tags from upstream
   ```bash
   $ git fetch upstream --tags
   ```

🆕 If you don't have the repo locally

1. Clone the KernelKit Linux kernel repository
   ```bash
   $ git clone git@github.com:kernelkit/linux.git
	```
2. Add the upstream remote
   ```bash
   $ git remote add upstream git://git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git
   ```

3. Checkout correct kernel branch
   ```bash
   $ git checkout kkit-linux-6.12.y
   ```

🛠  Continue from here (applies to both cases)


4. Rebase on the upstream release
   ```bash
   $ git rebase v6.12.29
   ```

6. Push changes and the tags
   ```bash

   $ git push -f origin kkit-linux-6.12.y --tags
   ```

**Move to your infix directory**

7. Generate patches
   ```bash
   $ make x86_64_defconfig
   $ cd output
   $ ../utils/kernel-refresh.sh -k /path/to/linux -o 6.12.28 -t v6.12.29
   ```
   > [!NOTE] See help of `kernel-refresh.sh` script for more information


8. Push changes
   Commit and push the changes. Don’t forget to update the s:changelog:doc/ChangeLog.md.

9. Create a pull request.
   > [!NOTE] Remember to set the pull request label to `ci:main` to ensure full CI coverage.


### Agree on YANG Model

When making changes to the `confd` and `statd` services, you will often need to update
the YANG models. If you are adding a new YANG module, it's best to follow the
structure of an existing one. However, before making any changes, **always discuss
them with the Infix core team**. This helps avoid issues later in development and
makes pull request reviews smoother.


Testing
-------

Manual testing can be done using Qemu by calling <kbd>make run</kbd>,
see also [Infix in Virtual Environments](virtual.md), or on a physical
device by upgrading to the latest build or "[netbooting](netboot.md)"
and running the image from RAM.  The latter is how most board porting
work is done -- **much quicker** change-load-test cycles.

The Infix automated test suite is built around Qemu and [Qeneth][2], see:

 * [Regression Testing with Infamy](testing.md)
 * [Docker Image](https://github.com/kernelkit/infix/blob/main/test/docker/README.md)

With any new feature added to Infix, it is essential to include relevant
test case(s).  See the [Test Development](testing.md#test-development)
section for guidance on adding test cases.


Reviewing
---------

While reviewing a pull request, you might find yourself wanting to play
around with a VM running that _exact_ version.  For such occasions,
[gh-dl-artifact.sh][8] is your friend in need!  It employs the [GitHub
CLI (gh)](https://cli.github.com) to locate a prebuilt image from our CI
workflow, download it, and prepare a local output directory from which
you can launch both `make run` instances, and run regression tests with
`make test` and friends.

For example, if you are curious about how PR 666 behaves in some
particular situation, you can use `gh` to switch to that branch, from
which `gh-dl-artifact.sh` can then download and prepare the
corresponding image for execution with our normal tooling:

    gh pr checkout 666
    ./utils/gh-dl-artifact.sh
    cd x-artifact-a1b2c3d4-x86_64
    make run

> [!NOTE]
> CI artifacts are built from a merge commit of the source and target
> branches.  Therefore, the version in the Infix banner will not match
> the SHA of the commit you have checked out.

Contributing
------------

Infix is built from many components, when contributing you need to set
up your own fork, create a local branch for your change, push to your
fork, and then use GitHub to create a *Pull Reqeuest*.

For this to work as *painlessly as possible* for everyone involved:

 1. Fork Infix to your own user or organization[^1]
 1. Fork all the Infix submodules, e.g., `kernelkit/buildroot` to your
    own user or organization as well
 1. Clone your fork of Infix to your laptop/workstation
 1. [Deactivate the Actions][6] you don't want in your fork
 1. Please read the [Contributing Guidelines][5] as well!

```bash
$ cd ~/Projects
$ git clone https://github.com/YOUR_USER_NAME/infix.git
...
$ cd infix/
$ git submodule update --init
...
```

> [!NOTE]
> When updating/synchronizing with upstream Infix changes you may have
> to synchronize your forks as well.  GitHub have a `Sync fork` button
> in the GUI for your fork for this purpose.  A cronjob on your server
> of choice can do this for you with the [GitHub CLI tool][7].

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
[5]: https://github.com/kernelkit/infix/blob/main/.github/CONTRIBUTING.md
[6]: https://docs.github.com/en/actions/managing-workflow-runs-and-deployments/managing-workflow-runs/disabling-and-enabling-a-workflow
[7]: https://cli.github.com/
[8]: https://github.com/kernelkit/infix/blob/main/utils/gh-dl-artifact.sh
