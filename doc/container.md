Containers in Infix
===================

* [Introduction](#introduction)
* [Caution](#caution)
* [Getting Started](#getting-started)
  * [Examples](#examples)
* [Networking and Containers](#networking-and-containers)
  * [CNI Bridge](#cni-bridge)
  * [CNI Host](#cni-host)
  * [Host Networking](#host-networking)
* [Example Containers](#example-containers)
  * [System Container](#system-container)
  * [Application Container: nftables](#application-container--nftables)
  * [Application Container: ntpd](#application-container--ntpd)
* [Upgrading a Container Image](#upgradeing-a-container-image)


Introduction
------------

Infix comes with native support for Docker containers using [podman][].
The [YANG model][1] describes the current level of support, complete
enough to run both system and application containers.

Key design features, like using Linux switchdev, allow users to assign
switch ports directly to containers, not just bridged VETH pairs, this
is a rare and in many cases *unique* feature of Infix.

All network specific settings are done using the IETF interfaces YANG
model, with augments for containers to ensure smooth integration with
the Container Network Interface ([CNI][]) that podman supports.


Caution
-------

A word of warning, containers can run on your system in privileged mode,
as `root`.  This gives them full access to devices on your system.  But
even when though unprivileged containers are fenced from the host with
Linux namespaces, and resource limited using Linux cgroups, which scope
container applications from seeing and accessing the complete system,
there is no guarantee that an application cannot ever break out of this
confinement.

 - If the system is compromised, containers can be used to easily
   install malicious software in your system and over the network
 - Your system is as secure as anything you run in the container
 - If you run containers, there is no security guarantee of any kind
 - Running 3rd party container images on your system could open a
   security hole/attack vector/attack surface
 - An expert with knowledge how to build exploits will be able to
   jailbreak/elevate to root even if best practices are followed

This being said, a system suspected of being compromised can always be
restored to a safe state with a factory reset.  Provided, of course,
that it has secure boot enabled.


Getting Started
---------------

In the CLI, containers can be run in one of two ways:

 1. `container run IMAGE [COMMAND]`, and
 2. enter `configure` context, then `edit container NAME`

The first is useful mostly for testing, or running single commands in an
image.  It is a wrapper for `podman run -it --rm ...`, while the latter
is a wrapper and adaptation of `podman create ...`.

The second create a container with a semi-persistent writable layer that
survives container restarts and host system restarts.  However, if you
change the container configuration or upgrade the image (see below), the
container will be recreated and the writable layer is lost.  This is why
it is recommended to set up a named volume for directories, or use file
mounts, in your container you want truly persistent content.

In fact, in many cases the best way is to create a `read-only` container
and use file mounts and volumes only for the critical parts.  Podman
ensures (using tmpfs) `read-only` containers still have writable
directories for certain critical file system paths: `/dev`, `/dev/shm`,
`/run`, `/tmp`, and `/var/tmp`.  Meaning, what you most often need is
writable volumes for `/var/lib` and `/etc`, or only file mounts for a
few files in `/etc`.  The actual needs depend on the container image and
application to run.

> **Note:** when running containers from public registries, double-check
> that they support the CPU architecture of your host system.  Remember,
> unlike virtualization, containers reuse the host's CPU and kernel.


### Examples

Classic Hello World:

    admin@example-c0-ff-ee:/> container run docker://hello-world

Persistent web server using nginx, sharing the host's network:

    admin@example-c0-ff-ee:/> configure
    admin@example-c0-ff-ee:/config> edit container web
    admin@example-c0-ff-ee:/config/container/web> set image docker://nginx:alpine
    admin@example-c0-ff-ee:/config/container/web> set publish 80:80
    admin@example-c0-ff-ee:/config/container/web> set host-network
    admin@example-c0-ff-ee:/config/container/web> leave
	admin@example-c0-ff-ee:/> show container

Exit to the shell and verify the service with curl, or try to attach
to your device's IP address using your browser:

    admin@example-c0-ff-ee:~$ curl http://localhost

or connect to port 80 of your running Infix system with a browser.  See
the following sections for how to add more interfaces and manage your
container at runtime.


Networking and Containers
-------------------------

By default, unlike other systems, persistent[^1] containers have no
networking enabled.  All network access has to be set up explicitly.
Currently two types of of [CNI][] networks are supported:

 - `cni-host`: one end of a VETH pair, or a physical Ethernet port
 - `cni-bridge`: an IP masquerading bridge


### CNI Bridge

All interface configuration is done in configure context.  Let's start
by creating an IP masquerading bridge, a common default for containers:

    admin@example-c0-ff-ee:/> configure
    admin@example-c0-ff-ee:/config> edit interface docker0
    admin@example-c0-ff-ee:/config/interface/docker0/> set type bridge
    admin@example-c0-ff-ee:/config/interface/docker0/> set container-network type cni-bridge
    admin@example-c0-ff-ee:/config/interface/docker0/> leave

We have to declare the interface type, and then also declare it as a
container network, ensuring the interface cannot be used by the system
for any other purpose.  E.g., a `cni-host` interface is supposed to be
used by a container, by declaring it as such we can guarantee that it
would never accidentally be added as a bridge or lag port.  Hence, to
move an interface currently set as a `bridge-port` it must be removed
from the bridge before being given to a container.

The default subnet for a `cni-bridge` is 172.17.0.0/16, the bridge will
take the `.1` address and hand out the rest of the range to containers
in a round-robin like fashion.  A container with this `network` get an
automatically created VETH pair connection to the bridge and a lot of
other networking parameters (DNS, default route) are set up.

Some of the defaults of a `cni-bridge` can be changed, e.g., instead of
`set container-network type cni-bridge`, above, do:

    admin@example-c0-ff-ee:/config/interface/docker0/> edit container-network
    admin@example-c0-ff-ee:/config/interface/docker0/container-network/> set type cni-bridge
    admin@example-c0-ff-ee:/config/interface/docker0/container-network/> edit subnet 192.168.0.0/16
    admin@example-c0-ff-ee:/config/interface/docker0/container-network/subnet/192.168.0.0/16/> set gateway 192.168.255.254
    admin@example-c0-ff-ee:/config/interface/docker0/container-network/subnet/192.168.0.0/16/> end
    admin@example-c0-ff-ee:/config/interface/docker0/container-network/> edit route 10.0.10.0/24
	admin@example-c0-ff-ee:/config/interface/docker0/container-network/route/10.0.10.0/24/> set gateway 192.168.10.254
	admin@example-c0-ff-ee:/config/interface/docker0/container-network/route/10.0.10.0/24/> end
	admin@example-c0-ff-ee:/config/interface/docker0/container-network/> end
    admin@example-c0-ff-ee:/config/interface/docker0/> leave

Other network settings, like DNS and domain, use built-in defaults in
CNI, but can be overridden from each container.  Other common settings
per container is the IP address and name of the network interface inside
the container.  The default, after each stop/start cycle, or reboot of
the host, is to name the interfaces `eth0`, `eth1`, in the order they
are given in the `network` list, and to give the container the next
address in a `cni-bridge`.  Below an example of a system container calls
`set network docker0`, here we show how to set options for that network:

    admin@example-c0-ff-ee:/config/container/ntpd/> edit network docker0 
    admin@example-c0-ff-ee:/config/container/ntpd/network/docker0/> 
    admin@example-c0-ff-ee:/config/container/ntpd/network/docker0/> set option 
    <string>  Options for CNI bridges.
    admin@example-c0-ff-ee:/config/container/ntpd/network/docker0/> help option 
    NAME
            option <string>
    
    DESCRIPTION
            Options for CNI bridges.
            Example: ip=1.2.3.4 to request a specific IP, both IPv4 and IPv6.
            interface_name=foo0 name to set interface name inside container.
    
    admin@example-c0-ff-ee:/config/container/ntpd/network/docker0/> set option ip=172.17.0.2
    admin@example-c0-ff-ee:/config/container/ntpd/network/docker0/> set option interface_name=wan
    admin@example-c0-ff-ee:/config/container/ntpd/network/docker0/> leave


### CNI Host

Another common use-case is to move a network interface into the network
namespace of a container.  Which the CNI bridge network does behind the
scenes with one end of the automatically created VETH pair.  This works
with regular Ethernet interfaces as well, but here we will use a VETH
pair as an example along with a regular bridge (where other Ethernet
interfaces may live as well).

    admin@example-c0-ff-ee:/config/> edit interface veth0
    admin@example-c0-ff-ee:/config/interface/veth0a/> set veth peer ntpd
    admin@example-c0-ff-ee:/config/interface/veth0a/> set ipv4 address 192.168.0.1 prefix-length 24
    admin@example-c0-ff-ee:/config/interface/veth0a/> end
    admin@example-c0-ff-ee:/config/> edit interface ntpd
    admin@example-c0-ff-ee:/config/interface/ntpd/> set ipv4 address 192.168.0.2 prefix-length 24
    admin@example-c0-ff-ee:/config/interface/ntpd/> set container-network
    admin@example-c0-ff-ee:/config/interface/ntpd/container-network/> set 

This is a routed setup, where we reserve 192.168.0.0/24 for the network
between the host and the `ntpd` container.  A perhaps more common case
is to put `veth0` as a port in a bridge with other physical ports.  The
point of the routed case is that port forwarding from the container in
this case is limited to a single interface, not *all interfaces* as is
the default in the CNI Bridge setup.


### Host Networking

The third use-case is host networking, this is where a container share
the network namespace of the host.  An example here could be a nftables
or ntpd container -- single applications which add core functionality to
the host operating system.

The host networking setup cannot be combined with any other network.

For an example, see below.


Example Containers
------------------

### System Container

Let's try out what we've learned by setting up a system container, a
container providing multiple services, using the `docker0` interface
we created previously:

    admin@example-c0-ff-ee:/> configure
    admin@example-c0-ff-ee:/config> edit container system
    admin@example-c0-ff-ee:/config/container/system/> set image ghcr.io/kernelkit/curios:edge
    admin@example-c0-ff-ee:/config/container/system/> set network docker0
    admin@example-c0-ff-ee:/config/container/system/> set publish 222:22
    admin@example-c0-ff-ee:/config/container/system/> leave

> **Note:** ensure you have a network connection to the registry.
> If the image cannot be pulled, creation of the container will be
> put in a queue and be retried every time there is a change in the
> routing table, e.g., default route is added.

Provided the image is downloaded successfully, a new `system` container
now runs behind the docker0 interface, forwarding container port 22 to
port 222 on all of the host's interfaces.  (See `help publish` in the
container configuration context for the full syntax.)

Available containers can be accessed from admin-exec:

    admin@example-c0-ff-ee:/> show container 
    CONTAINER ID  IMAGE                          COMMAND     CREATED       STATUS       PORTS                 NAMES
    439af2917b44  ghcr.io/kernelkit/curios:edge              41 hours ago  Up 16 hours  0.0.0.0:222->222/tcp  system

This is a system container, so you can "attach" to it by starting a
shell (or logging in with SSH):

    admin@example-c0-ff-ee:/> container shell system
	root@439af2917b44:/# 

Notice how the hostname inside the container changes.  By default the
container ID (hash) is used, but this can be easily changed:

    root@439af2917b44:/# exit
	admin@infix-00-00-00:/> configure 
	admin@infix-00-00-00:/config/> edit container system 
	admin@infix-00-00-00:/config/container/system/> set hostname system1
	admin@infix-00-00-00:/config/container/system/> leave
	admin@infix-00-00-00:/> container shell system
	root@system1:/# 

[^1]: this does not apply to the admin-exec command `container run`.
    This command is intended to be used for testing and evaluating
	container images.  Such containers are given a private network
	behind an IP masquerading bridge.


### Application Container: nftables

Infix currently does not have a native firewall configuration, and even
when it does it will never expose the full capabilities of `nftables`.
For really advanced setups, the following will be the only alternative:

    admin@example-c0-ff-ee:/> configure
    admin@example-c0-ff-ee:/config> edit container nftables
    admin@example-c0-ff-ee:/config/container/system/> set image ghcr.io/kernelkit/curios-nftables:edge
    admin@example-c0-ff-ee:/config/container/system/> set host-network
    admin@example-c0-ff-ee:/config/container/system/> edit file nftables.conf
    admin@example-c0-ff-ee:/config/container/system/file/nftables.conf/> set path /etc/nftables.conf
    admin@example-c0-ff-ee:/config/container/system/file/nftables.conf/> set content
    ... interactive editor starts up where you can paste your rules ...
    admin@example-c0-ff-ee:/config/container/system/file/nftables.conf/> leave


### Application Container: ntpd

The default NTP server/client in Infix is Chrony, a fully working and
capable workhorse for most use-cases.  However, it does not support a
feature like multicasting, for that you need ISC ntpd.

As we did with `nftables`, previously, we can use host networking and
set up a read-only config file that is bind-mounted into the container's
file system and store in the host's `startup-config`.  However, `ntpd`
also saves clock drift information in `/var/lib/ntpd`, so we will also
use volumes in this example.

> Infix support named volumes (only), and it is not possible to share a
> volume between containers.  All the tricks possible with volumes may
> be added in a later release.

A volume is an automatically created read-writable area that follows the
life of your container.  They survive reboots and upgrading of the base
image, unlike the persistent writable layer you get by default, which
does not survive upgrades.  The volume is created by podman when the
container first starts up, unlike a regular bind mount it synchronizes
with the contents of the underlying container image's path on the first
start.  I.e., "bind-mount, if empty: then rsync".

    admin@example-c0-ff-ee:/> configure
    admin@example-c0-ff-ee:/config> edit container ntpd
    admin@example-c0-ff-ee:/config/container/ntpd/> set image ghcr.io/kernelkit/curios-ntpd:edge
    admin@example-c0-ff-ee:/config/container/ntpd/> set network ntpd              # From veth0 above
    admin@example-c0-ff-ee:/config/container/ntpd/> edit file ntp.conf
    admin@example-c0-ff-ee:/config/container/ntpd/file/ntp.conf/> set path /etc/ntp.conf
    admin@example-c0-ff-ee:/config/container/ntpd/file/ntp.conf/> set content
    ... interactive editor starts up where you can paste your rules ...
    admin@example-c0-ff-ee:/config/container/ntpd/file/ntp.conf/> end
    admin@example-c0-ff-ee:/config/container/ntpd/> edit volume varlib
    admin@example-c0-ff-ee:/config/container/ntpd/volume/varlib/> set path /var/lib
    admin@example-c0-ff-ee:/config/container/ntpd/volume/varlib/> leave
    admin@example-c0-ff-ee:/> copy running-config startup-config

The `ntp.conf` file is stored in the host's `startup-config` and any
state data in the container's `/var/lib` is retained between reboots
and across image upgrades.


Upgrading a Container Image
---------------------------

All container configurations are locked to the image hash at the time of
first download, not just ones that use an `:edge` or `:latest` tag.  An
upgrade of containers using versioned images is more obvious -- update
the configuration -- but the latter is a bit trickier.  Either remove
the configuration and recreate it (leave/apply the changes between), or
use the admin-exec level command:

    admin@example-c0-ff-ee:/> container upgrade NAME

Where `NAME` is the name of your container.  This command stops your
container, does a `container pull IMAGE`, and then recreates the
container with the new image.  Upgraded containers are not automatically
restarted.

    admin@example-c0-ff-ee:/> container start NAME

> **Note:** the default writable layer is lost when upgrading the image
> Use named volumes for directories with writable content you wish to
> keep over an upgrade.


[1]:      https://github.com/kernelkit/infix/blob/main/src/confd/yang/infix-containers%402023-12-14.yang
[CNI]:    https://www.cni.dev/
[podman]: https://podman.io
