Infix Variants
==============

Infix has two main *flavors*.  Both have a default `admin` account,
which is allowed to log in from remote, default password `admin` --
*customer specific builds* may have something else, e.g., per-device
generated factory password.

> See [Infix Discovery](discovery.md) to locate your device.


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

[^1]: A [RESTCONF][] based WebUI is also in progress.


Classic
-------

Infix Classic is very much like a traditional embedded Linux system.  It
use the same kernel as NETCONF builds, but unlike them it is up to the
administrator to manually modify system configuration files in `/etc`
and control the system services using the `initctl` tool.

For example, networking is configured by editing the [ifupdown-ng][6]
files in `/etc/network/interfaces`.

> In Classic builds the `/etc` directory is saved across reboots.

To perform a factory reset, wiping all changes in `/etc`, and all other
areas of the file system that are persistent, use the <kbd>factory</kbd>
tool.

See the online <kbd>help</kbd> command for an introduction to the system
and help on available tools, like text editors, network debugging, etc.


Hybrid Mode
-----------

Since Infix is under heavy development, it does not yet have all bells
and whistles in place in the NETCONF builds.  To that end it is possible
to manually manage certain properties and services.  It's a little bit
tricky since any changes to the `/etc` directory is lost at reboot.

To work around that we use the [run-parts(8)][] feature of the system,
available in some customer specific images.  The system runs any user
scripts in `/cfg/start.d` before leaving runlevel S (bootstrap).

### Starting OSPF

For example, the following starts OSPF:

```sh
root@infix:~$ cp -a /etc/frr /cfg/
root@infix:~$ mkdir /cfg/start.d
root@infix:~$ cd /cfg/start.d
root@infix:/cfg/start.d$ cat <<EOF >10-enable-ospf.sh
#!/bin/sh
# Use vtysh to modify the OSPF configuration
mount --bind /cfg/frr /etc/frr
initctl enable zebra
initctl enable ospfd
initctl enable bfdd
(sleep 1; vtysh -b) &
exit 0
EOF
root@infix:/cfg/start.d$ chmod +x 10-enable-ospf.sh
```

The `/cfg` area is persistent across reboots.  Here we assume the user
has already created the `/cfg/frr` directory, populated it with the
original files from `/etc/frr`, and then modified the appropriate files
to enable OSPF and BFD.

### Starting Containers

Using `/cfg/start.d` is also the way to start containers (provided the
images have been downloaded with `podman pull` first):

```
root@infix:/cfg/start.d$ cat <<EOF >20-enable-container.sh
#!/bin/sh
podman-service -e -d "Nginx container" -p "-p 80:80 -v /cfg/www:/usr/share/nginx/html:ro" nginx:alpine
exit 0
EOF
root@infix:/cfg/start.d$ chmod +x 20-enable-container.sh
```

Reboot to activate the changes.  To activate the changes without
rebooting, run the script and call `initctl reload`.

For more information, see [Containers in Infix](container.md).

> **Note:** Neither [Frr](https://frrouting.org) (Zebra/OSPF/BFD) or
> [podman](https://podman.io) are enabled in the official Infix builds.
> Some customers have them enabled in their specific builds, and you can
> of course also enable it yourself in Infix by using `make menuconfig`
> followed by rebuilding the image.


[1]: https://www.sysrepo.org/
[2]: https://github.com/CESNET/netopeer
[3]: https://pypi.org/project/netconf-client/
[4]: http://www.seguesoft.com/index.php/netconfc/
[5]: https://www.mg-soft.si/mgNetConfBrowser.html
[6]: https://github.com/ifupdown-ng/ifupdown-ng
[run-parts(8)]: https://manpages.ubuntu.com/manpages/trusty/man8/run-parts.8.html
[RESTCONF]: https://datatracker.ietf.org/doc/html/rfc8040

