Welcome to Infix!
=================

Nice to meet ❤️ you!  If you are reading this then you have possibly
just downloaded and unpacked a release build and are curious about how
to proceed from here.

To test Infix You need a Linux 🐧 system with Qemu or GNS3 installed.
We recommend Debian based systems, like Ubuntu and Linux Mint.

> For a pain-free experience we recommend enabling CPU virtualization in
> your BIOS/UEFI, which for many computers is disabled by default.

From this point we assume you have your x86_64/AMD64 based Linux system
up and running.  Time to start your favorite terminal application! 😃


Installing Qemu
---------------

This README focus on getting you started with Qemu.  From a terminal,
install (at least) the x86/x86_64 emulator.  The 'virt-manager' is a
package that helps pull in other dependencies you may need:

    $ sudo apt install qemu-system-x86 virt-manager

That's it.


Running Infix in Qemu
---------------------

Depending on how your Linux installation is set up, the following may
require being run with superuser privileges, i.e., you may need to
repend the command with 'sudo'.

    $ ./qemu.sh

You should now see the Infix init system booting up.  When the final
"Please press Enter to activate this console." is shown, press Enter
and the login: prompt is displayed.

The default credentials for the demo builds is

    login: admin
    password: admin

    Infix -- a Network Operating System v24.09.0-rc1 (hvc0)
    infix-00-00-00 login: admin
    Password: 
    .-------.
    |  . .  | Infix -- a Network Operating System
    |-. v .-| https://kernelkit.org
    '-'---'-'

    Run the command 'cli' for interactive OAM

    admin@infix-00-00-00:~$

You're in!  Play around in your sandbox as much as you like, if you
run into problems or have questions, please see the documentation,
and don't hesitate to get in touch with us! 😃

 - https://github.com/kernelkit/infix/tree/main/doc


Customizing your "Hardware"
---------------------------

For more Ethernet ports in your emulated system you need to change the
Qemu configuration used for Infix.  This can be done using a menuconfig
interface, which requires the following extra package:

    $ sudo apt install kconfig-frontends

We can now enter the configuration:

    $ ./qemu.sh -c

Go down to *Networking*, select *TAP*, now you can change the *Number of
TAPs*, e.g. to 10.  Exit and save the configuration, then you can start
Qemu again:

   ./qemu.sh

> Make sure to do a factory reset from the CLI, otherwise you will be
> stuck with that single interface from before.


Errors on Console
-----------------

If you see the following line printed one or more times, don't panic.

    LABEL=var: Can't lookup blockdev

See the Customizing section above.  To silence the error you need to
create another writable partition for Infix to store logs, container
images, etc.  Look for the 'var' keyword, you can adjust the size of
the partition.


Graphical Network Simulator 3 (GNS3)
------------------------------------

GNS3 is a very powerful front-end to Qemu which takes care of creating
virtual links between network devices running in Qemu.  This README is
only link to the material you need.  This directory holds the appliance
file, .gns3a, that references image files also in this directory, that
you need to load into GNS3.

The necessary extra packages are available through the offical PPA.  If
you don't know what a PPA is, read up on that first:

 - https://launchpad.net/~gns3/+archive/ubuntu/ppa

There's a lot of tutorials and guides online, start here:

 - https://docs.gns3.com/docs/


About
-----

Infix is a free, Linux based, immutable Network Operating System (NOS)
built on Buildroot, and sysrepo.  A powerful mix that ease porting to
different platforms, simplify long-term maintenance, and provide easy
management using NETCONF, RESTCONF, or the built-in command line
interface (CLI) from a console or SSH login.

