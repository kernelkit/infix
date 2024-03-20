Infix Variants
==============

Infix comes in two *flavors*.  Both have a default `admin` account,
allowed to log in from remote, with the default password `admin` --
*customer specific builds* may have something else, e.g., per-device
generated factory password.


NETCONF
-------

Infix use by [sysrepo][1] and [Netopeer][2] to provide NETCONF support.
A set of sysrepo plugins configure the network, using the Linux iproute2
tool suite, generate configuration files in `/etc`, and control the all
system daemons, e.g., enable DHCP client on an interface.

Configuration of an Infix device can be done remotely, using command
line tools like [netconf-client][3] and [netopeer2-cli][2], or desktop
GUI tools like [NETCONFc][4] and [MG-SOFT NetConf Browser][5].  It is
also possible to log in to the device using SSH and set it up locally
using the built-in [CLI](cli/introduction.md)[^1].

> **Note:** unlike Infix Classic, the `/etc` directory is a volatile RAM
> disk populated on each boot from the `startup-config`, ensuring a
> coherent centralized view of the system.

For more information, see [Containers in Infix](container.md).

[^1]: A [RESTCONF][7] based WebUI is also in progress.


Classic
-------

Infix Classic is very much like a traditional embedded Linux system.
Stripped down, single read-only image, reusing the same Linux kernel and
Buildroot base as the NETCONF variant.  Unlike the NETCONF variant, it
is up to the administrator to manually modify system configuration files
in `/etc`[^2] and control system services using the `initctl` tool.

For example, networking is configured by editing the [ifupdown-ng][6]
files in `/etc/network/interfaces`.

To perform a factory reset, wiping all changes in `/etc`, and all other
areas of the file system that are persistent, use the <kbd>factory</kbd>
tool.

See the online <kbd>help</kbd> command for an introduction to the system
and help on available tools, like text editors, network debugging, etc.

> **Note:** the Classic builds are legacy at this point.  They were
> initially used for educational purposes, and sometimes as slightly
> more useful end-devices in GNS3.  Little to no testing is done on
> them and they may eventually be migrated to a separate repository.

[^2]: In Classic builds the `/etc` directory is saved across reboots on
	a separate read-write partition.

[1]: https://www.sysrepo.org/
[2]: https://github.com/CESNET/netopeer
[3]: https://pypi.org/project/netconf-client/
[4]: http://www.seguesoft.com/index.php/netconfc/
[5]: https://www.mg-soft.si/mgNetConfBrowser.html
[6]: https://github.com/ifupdown-ng/ifupdown-ng
[7]: https://datatracker.ietf.org/doc/html/rfc8040

