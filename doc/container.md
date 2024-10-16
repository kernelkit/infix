Containers in Infix
===================
<img align="right" src="img/docker.webp" alt="Docker whale" width=360>

* [Introduction](#introduction)
* [Caution](#caution)
* [Getting Started](#getting-started)
  * [Examples](#examples)
* [Container Images](#container-images)
* [Upgrading a Container Image](#upgrading-a-container-image)
* [Networking and Containers](#networking-and-containers)
  * [Container Bridge](#container-bridge)
  * [Container Host Interface](#container-host-interface)
  * [Host Networking](#host-networking)
* [Mounts and Volumes](#mounts-and-volumes)
  * [Content Mounts](#content-mounts)
* [Example Containers](#example-containers)
  * [System Container](#system-container)
  * [Application Container: nftables](#application-container-nftables)
  * [Application Container: ntpd](#application-container-ntpd)
* [Advanced](#advanced)
  * [Running Host Commands From Container](#running-host-commands-from-container)

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
container networking in podman.

> **Note:** even though the `podman` command can be used directly from a
> shell prompt, we strongly recommend using the CLI commands instead.
> They employ the services of a wrapper `container` script which handles
> the integration of containers in the system.


Caution
-------

A word of warning.  Containers can run on your system in privileged
mode, as `root`, giving them full access to devices on your system.
Even though containers are fenced from the host with Linux namespaces,
resource limited using cgroups, and normally run with capped privileges,
a privileged container is relatively easy to break out of.  A trivial
example is given in the [Advanced](#advanced) section of this document.

We recommend avoiding privileged containers, if possible (they do have
valid use-cases) and instead use [capabilities](#capabilities).

Remember:

 - If the system is compromised, containers can be used to easily
   install malicious software in your system and over the network
 - Your system is as secure as anything you run in the container
 - If you run containers, there is no security guarantee of any kind
 - Running 3rd party container images on your system could open a
   security hole/attack vector/surface
 - An expert with knowledge how to build exploits will be able to
   jailbreak/elevate to root even if best practices are followed

This being said, a system suspected of being compromised can always be
restored to a safe state with a factory reset.  Provided, of course,
that it has secure boot enabled.


Getting Started
---------------

In the CLI, containers can be run in one of two ways:

 1. `container run IMAGE [COMMAND]`, or
 2. enter `configure` context, then `edit container NAME`

The former is useful mostly for testing, or running single commands in
an image.  It is a wrapper for `podman run -it --rm ...`, while the
latter is a wrapper and adaptation of `podman create ...`.

The second create a container with a semi-persistent writable layer that
survives container restarts and host system restarts.  However, if you
change the container configuration or upgrade the image (see below), the
container will be recreated and the writable layer is lost.  This is why
it is recommended to set up a named volume for directories, or use file
[Content Mounts](#content-mounts), in your container if you want truly
persistent content.

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


<img align="right" src="img/docker-hello-world.svg" alt="Hello World" width=360>

### Examples

Classic Hello World:

    admin@example:/> container run docker://hello-world
    Starting docker://hello-world :: use Ctrl-p Ctrl-q to detach
    Trying to pull docker.io/library/hello-world:latest...
    Getting image source signatures
    Copying blob c1ec31eb5944 done
    Copying config d2c94e258d done
    Writing manifest to image destination
    Storing signatures

    Hello from Docker!
    This message shows that your installation appears to be working correctly.

Persistent web server using nginx, sharing the host's network:

    admin@example:/> configure
    admin@example:/config> edit container web
    admin@example:/config/container/web> set image docker://nginx:alpine
    admin@example:/config/container/web> set publish 80:80
    admin@example:/config/container/web> set network host
    admin@example:/config/container/web> leave
    admin@example:/> show container

Exit to the shell and verify the service with curl, or try to attach
to your device's IP address using your browser:

    admin@example:~$ curl http://localhost

or connect to port 80 of your running Infix system with a browser.  See
the following sections for how to add more interfaces and manage your
container at runtime.


Container Images
----------------

The underlying `podman` project supports importing and fetching images in
a variety of ways, the most common ones are also supported by Infix.  In
this section we present how to use them and in the next section we show
how to upgrade to a newer base image.

The CLI help shows:

    admin@example:/config/container/system/> help image
    NAME
        image <string>

    DESCRIPTION
        Docker image for the container: [transport]name[:tag|@digest]

        quay.io/username/myimage     -- Pull myimage:latest
        docker://busybox             -- Pull busybox:latest from Docker Hub
        docker://ghcr.io/usr/img     -- Pull img:latest from GitHub packages
        dir:/media/usb/myimage:1.1   -- Use myimage v1.1 from USB media
        docker-archive:/tmp/archive  -- Use archive:latest from tarball
        oci-archive:/lib/oci/archive -- Use archive:latest from OCI archive
                                        May be in .tar or .tar.gz format

        Note: if a remote repository cannot be reached, the creation of the
              container will be put on a queue that retries pull every time
              there is a route change in the host's system.

> **Note::** the built-in help system in the CLI is generated from the
> YANG model, so the same information is also available for remote
> NETCONF users.

The two most common variants are `docker://` and `oci-archive:/`.

The former requires a working Docker registry and the latter operates on
a plain OCI archive.  Infix does not come with a built-in registry, so
the `docker://` option is best used with external services, which in
turn require [networking][0] to be up.  In a deployment phase the
easiest may be to set up a single interface on your host system with
DHCP client.

The default method is `docker://`, so when setting the `image` for your
container, you can omit the `docker://` prefix.  You can also use the
admin-exec command `container pull docker://...`, and when configuring a
container `podman` will check first if it has the image before trying to
download anything.  (See also the upgrade section, below.)

The `oci-archive:/` is interesting since many users may not have, or do
not want to, publish their images in a registry.  Use the Docker [OCI
exporter][5] or any other tool that supports generating [OCI Image][3]
format.  Infix supports loading both `.tar` or `.tar.gz` formats.

Here we show a simple example of fetching an OCI image to the system,
but many others exist, tools like `wget`, `curl`, and `scp` come to
mind.

**Shell OCI Example:**

    admin@example:~$ cd /var/tmp/
    admin@example:/var/tmp$ sudo wget https://github.com/kernelkit/curiOS/releases/download/edge/curios-oci-amd64.tar.gz
    Connecting to github.com (140.82.121.3:443)
    wget: note: TLS certificate validation not implemented
    Connecting to objects.githubusercontent.com (185.199.109.133:443)
    saving to 'curios-oci-amd64.tar.gz'
    curios-oci-amd64.tar 100% |*********************************| 7091k  0:00:00 ETA
    'curios-oci-amd64.tar.gz' saved
    admin@example:/var/tmp$ ll
    total 7104
    drwxr-xr-x    3 root     root          4096 Mar 27 14:22 ./
    drwxr-xr-x   14 root     root          4096 Mar 27 11:57 ../
    -rw-r--r--    1 root     root       7261785 Mar 27 14:22 curios-oci-amd64.tar.gz
    drwx------    6 frr      frr           4096 Mar 27 11:57 frr/

Importing the image into podman can be done either from the CLI
admin-exec context ...

    admin@example:/var/tmp$ cli
    admin@example:/> container load /var/tmp/curios-oci-amd64.tar.gz name curios:edge

> The `name curios:edge` is the tag you give the imported
> (raw) archive which you can then reference in your container image
> configuration: `set image curios:edge`.

... or by giving the container configuration the full path to the OCI
archive, which helps greatly with container upgrades (see below):

    admin@example:/config/container/system/> set image oci-archive:/var/tmp/curios-oci-amd64.tar.gz


Upgrading a Container Image
---------------------------
<img align="right" src="img/shield-checkmark.svg" alt="Hello World" width=100>

The applications in your container are an active part of the system as a
whole, so make it a routine to keep your container images up-to-date!

> **Note:** the default writable layer is lost when upgrading the image.
> Use named volumes for content that you want to persist across upgrades.

All container configurations are locked to the image hash at the time of
first download, not just ones that use an `:edge` or `:latest` tag.  An
upgrade of containers using versioned images is more obvious -- update
the configuration to use the new `image:tag` -- the latter is a bit
trickier.  Either remove the configuration and recreate it (leave/apply
the changes between), or use the admin-exec level command:

    admin@example:/> container upgrade NAME

Where `NAME` is the name of your container.  This command stops the
container, does `container pull IMAGE`, and then recreates it with the
new image.  Upgraded containers are automatically restarted.

**Example using registry:**

	admin@example:/> container upgrade system
	system
	Trying to pull ghcr.io/kernelkit/curios:edge...
	Getting image source signatures
	Copying blob 07bfba95fe93 done
	Copying config 0cb6059c0f done
	Writing manifest to image destination
	Storing signatures
	0cb6059c0f4111650ddbc7dbc4880c64ab8180d4bdbb7269c08034defc348f17
	system: not running.
	59618cc3c84bef341c1f5251a62be1592e459cc990f0b8864bc0f5be70e60719

An OCI archive image can be upgraded in a similar manner, the first step
is of course to get the new archive onto the system (see above), and
then, provided the `oci-archive:/path/to/archive` format is used, call
the upgrade command as

	admin@example:/> container upgrade system
	Upgrading container system with local archive: oci-archive:/var/tmp/curios-oci-amd64.tar.gz ...
	7ab4a07ee0c6039837419b7afda4da1527a70f0c60c0f0ac21cafee05ba24b52


Capabilities
-------------

An unprivileged container works for almost all use-cases, but there are
occasions where they are too restricted and users being looking for the
`privileged` flag.  Capabilities offers a middle ground.

For example, a system container from which `ping` does not work:

	admin@example:/config/container/system/> edit capabilities
	admin@example:/config/container/system/capabilities/> set add net_raw
	admin@example:/config/container/system/capabilities/> end
	admin@infix-00-00-00:/config/container/system/> show
	...
	capabilities {
	  add net_raw;
	}
	...

Infix supports a subset of all [capabilities][6] that are relevant for
containers.  Please note, that this is and advanced topic and will
require time and analysis of your container application to figure out
which capabilities you need.


Networking and Containers
-------------------------

By default, unlike other systems, persistent[^1] containers have no
networking enabled.  All network access has to be set up explicitly.
Currently two types of of container networks are supported:

 - `host`: one end of a VETH pair, or a physical Ethernet port
 - `bridge`: an IP masquerading bridge

> For more information on VETH pairs, see the [Networking Guide][0].


### Container Bridge

A container bridge is what most container setups use and users want.

![IP Masquerading Container Bridge](img/cni-bridge-firewall.svg)

The difference from a regular bridge is that the container runtime fully
manages them -- connecting containers with automatically created VETH
pairs (look at the bridge port names) and setting up firewall rules
between the host and other containers, as well as managing port
forwarding.  This transparent background management is what makes
container use seem to be so simple.

All interface configuration is done in configure context.

    admin@example:/> configure
    admin@example:/config> edit interface docker0
    admin@example:/config/interface/docker0/> set container-network
    admin@example:/config/interface/docker0/> leave

There is more to this story.  When using the CLI, and sticking to common
interface nomenclature, Infix helps you with some of the boring stuff.
E.g., creating a new interface with a name like `brN` or `dockerN`
automatically *infers* the interface types, which you would otherwise
have to set manually:

    admin@example:/config/interface/docker0/> set type bridge
    admin@example:/config/interface/docker0/> set container-network type bridge

> **Note:** when doing the same operation over NETCONF there is no
> inference, so all the "magic" settings need to be defined.  This
> makes the CLI very useful for first setup and then extracting the
> resulting XML from the shell using the `cfg -X` command.

We have to declare the interface as a container network, ensuring the
interface cannot be used by the system for any other purpose.  E.g., a
container `host` interface is supposed to be used by a container, by
declaring it as such we can guarantee that it would never accidentally
be added as a bridge or lag port.  Hence, to move an interface currently
set as a `bridge-port` it must be removed from the bridge before being
given to a container.

The default subnet for a container `bridge` is 172.17.0.0/16, the bridge
takes the `.1` address and hand out the rest of the range to containers
in a round-robin like fashion.  A container with this `network` get an
automatically created VETH pair connection to the bridge and a lot of
other networking parameters (DNS, default route) are set up.

Some of the defaults of a container `bridge` can be changed, e.g.,
instead of `set container-network type bridge`, above, do:

    admin@example:/config/interface/docker0/> edit container-network
    admin@example:/config/interface/docker0/container-network/> set type bridge
    admin@example:/config/interface/docker0/container-network/> edit subnet 192.168.0.0/16
    admin@example:/config/interface/docker0/container-network/subnet/192.168.0.0/16/> set gateway 192.168.255.254
    admin@example:/config/interface/docker0/container-network/subnet/192.168.0.0/16/> end
    admin@example:/config/interface/docker0/container-network/> edit route 10.0.10.0/24
	admin@example:/config/interface/docker0/container-network/route/10.0.10.0/24/> set gateway 192.168.10.254
	admin@example:/config/interface/docker0/container-network/route/10.0.10.0/24/> end
	admin@example:/config/interface/docker0/container-network/> end
    admin@example:/config/interface/docker0/> leave

Other network settings, like DNS and domain, use built-in defaults, but
can be overridden from each container.  Other common settings per
container is the IP address and name of the network interface inside the
container.  The default, after each stop/start cycle, or reboot of the
host, is to name the interfaces `eth0`, `eth1`, in the order they are
given in the `network` list, and to give the container the next address
in a `bridge`.  Below an example of a system container calls `set
network interface docker0`, here we show how to set options for that
network:

    admin@example:/config/container/ntpd/> edit network docker0 
    admin@example:/config/container/ntpd/network/docker0/> 
    admin@example:/config/container/ntpd/network/docker0/> set option 
    <string>  Options for masquerading container bridges.
    admin@example:/config/container/ntpd/network/docker0/> help option 
    NAME
            option <string>
    
    DESCRIPTION
            Options for masquerading container bridges.

            Example: ip=1.2.3.4            -- request a specific IP (IPv4 or IPv6)
                     mac=00:01:02:c0:ff:ee -- set fixed MAC address in container
                     interface_name=foo0   -- set interface name inside container
    
    admin@example:/config/container/ntpd/network/docker0/> set option ip=172.17.0.2
    admin@example:/config/container/ntpd/network/docker0/> set option interface_name=wan
    admin@example:/config/container/ntpd/network/docker0/> leave


### Container Host Interface

Another common use-case is to move network interfaces into the network
namespace of a container[^3].  This of course works with plain Ethernet
interfaces as well, but here we will use one end of a VETH pair as an
example.

The network `option` setting is available also for this case, but only
the `interface_name=foo0` option works.  Which is still very useful.  To
change the MAC address, you need to use the `custom-phys-address` in the
general network settings.

[^3]: Something which the container bridge network type does behind the
    scenes with one end of an automatically created VETH pair.


#### Routed Setup

In this routed setup we reserve 192.168.0.0/24 for the network between
the host and the `ntpd` container.

![Basic VETH pair connecting ntpd container.](img/cni-ntpd-routed.svg)

Configuration is a straight-forward VETH pair setup where we name the
container-end of pair `ntpd`.  This is just a convenience for us when
reading the configuration later.  The *real action* happens on the last
line where we declare the `ntpd` end as a container network interface:

    admin@example:/config/> edit interface veth0
    admin@example:/config/interface/veth0/> set veth peer ntpd
    admin@example:/config/interface/veth0/> set ipv4 address 192.168.0.1 prefix-length 24
    admin@example:/config/interface/veth0/> end
    admin@example:/config/> edit interface ntpd
    admin@example:/config/interface/ntpd/> set ipv4 address 192.168.0.2 prefix-length 24
    admin@example:/config/interface/ntpd/> set custom-phys-address static 00:c0:ff:ee:00:01
    admin@example:/config/interface/ntpd/> set container-network

> Notice how you can also set a custom MAC address at the same time.

Adding the interface to the container is the same as before, but since
everything for host interfaces is set up in the interfaces context, we
can take a bit of a shortcut.

    admin@example:/config/container/ntpd/> set network interface ntpd
    admin@example:/config/container/ntpd/> leave

> Use the `set network interface ntpd option interface_name=foo0` to set
> the name of the interface inside the container to `foo0`.

The point of the routed case is that port forwarding from the container
in this case is limited to a single interface, not *all interfaces* as
is the default in the masquerading container bridge setup.


#### Bridged Setup

A perhaps more common case is to bridge the other end of the VETH pair
with other physical ports.  In this section we show how to add a new
pair to give our container two interfaces:

![Bridged setup with ntpd container.](img/cni-ntpd-bridged.svg)

We start by adding the second VETH pair:

    admin@example:/config/> edit interface veth1a
    admin@example:/config/interface/veth1a/> set veth peer veth1
    admin@example:/config/interface/veth1a/> set ipv4 address 192.168.1.2 prefix-length 24

> The LAN bridge (br1) in this example has IP address 192.168.1.1.

When a container has multiple host interfaces it can often be useful to
have a default route installed.  This can be added from the host with a
`0.0.0.0/0` route on one of the interfaces:

    admin@example:/config/interface/veth1a/> set container-network route 0.0.0.0/0 gateway 192.168.1.1
    admin@example:/config/interface/veth1a/> show
    type veth;
	container-network {
	  type host;
	  route 0.0.0.0/0 {
		gateway 192.168.1.1;
	  }
	}
	veth {
	  peer veth1;
	}
    admin@example:/config/interface/veth1a/> end
    admin@example:/config/> set interface veth1 bridge-port bridge br1

Please note, container network routes require the base interface also
have a static IP address set.  Setting only the route, but no address,
means the route is skipped.


### Host Networking

The third use-case is host networking, this is where a container share
the network namespace of the host.  An example here could be a nftables
or ntpd container -- single applications which add core functionality to
the host operating system.

The host networking setup cannot be combined with any other network.

For an example, [see below](#application-container-nftables).


Mounts and Volumes
------------------

It is possible to mount files, directories, and even files matching a
glob, into a container.  This gives precise control over the container's
file system:

    admin@example:/config/container/system/> edit mount leds
    admin@example:/config/container/system/mount/leds> set source /sys/class/leds
    admin@example:/config/container/system/mount/leds> set target /sys/class/leds
    admin@example:/config/container/system/mount/leds> end
    admin@example:/config/container/system/>

Sometimes *volumes* are a better fit.  A volume is an automatically
created read-writable entity that follows the life of your container.

    admin@example:/config/container/ntpd/> set volume varlib target /var/lib

Volumes survive reboots and upgrading of the base image, unlike the
persistent writable layer you get by default, which does not survive
upgrades.  The volume is created by podman when the container first
starts up, unlike a regular bind mount it synchronizes with the contents
of the underlying container image's path on the first start.  I.e.,
"bind-mount, if empty: then rsync".

> Infix support named volumes (only), and it is not possible to share a
> volume between containers.  All the tricks possible with volumes may
> be added in a later release.

### Content Mounts

Content mount is a special type of where the file contents for the
container is stored alongside the container configuration.  This can be
very useful when deploying similar systems at multiple sites.  When the
host loads its `startup-config` (or even `factory-config`) a temporary
file is created using the decoded base64 data from the `content` node.

    admin@example:/config/container/ntpd/> edit mount ntpd.conf
    admin@example:/config/container/ntpd/mount/ntpd.conf> text-editor content
    ... interactive editor starts up ...
    admin@example:/config/container/ntpd/mount/ntpd.conf> set target /etc/ntpd.conf
    admin@example:/config/container/ntpd/mount/ntpd.conf> end
    admin@example:/config/container/ntpd/>

The editor is a small [Emacs clone called Mg][2], see the built-in help
text, or press Ctrl-x Ctrl-c to exit and save.  When the editor exits
the contents are base64 encoded and stored in the candidate datastore.

> **Note:** since these files are always recreated when the host is
> restarted, changes made by the container are not preserved, or saved
> back to the host's startup-config even if the read-only option is off.

Infix has three different text editors available.  For more information,
see [CLI Text Editor](cli/text-editor.md).


Example Containers
------------------

### System Container

Let's try out what we've learned by setting up a system container, a
container providing multiple services, using the `docker0` interface
we created previously:

    admin@example:/> configure
    admin@example:/config> edit container system
    admin@example:/config/container/system/> set image ghcr.io/kernelkit/curios:edge
    admin@example:/config/container/system/> set network interface docker0
    admin@example:/config/container/system/> set publish 222:22
    admin@example:/config/container/system/> leave

> **Note:** ensure you have a network connection to the registry.
> If the image cannot be pulled, creation of the container will be
> put in a queue and be retried every time there is a change in the
> routing table, e.g., default route is added.

Provided the image is downloaded successfully, a new `system` container
now runs behind the docker0 interface, forwarding container port 22 to
port 222 on all of the host's interfaces.  (See `help publish` in the
container configuration context for the full syntax.)

Available containers can be accessed from admin-exec:

    admin@example:/> show container
    CONTAINER ID  IMAGE                          COMMAND     CREATED       STATUS       PORTS                 NAMES
    439af2917b44  ghcr.io/kernelkit/curios:edge              41 hours ago  Up 16 hours  0.0.0.0:222->222/tcp  system

This is a system container, so you can "attach" to it by starting a
shell (or logging in with SSH):

    admin@example:/> container shell system
    root@439af2917b44:/#

Notice how the hostname inside the container changes.  By default the
container ID (hash) is used, but this can be easily changed:

    root@439af2917b44:/# exit
    admin@example:/> configure
    admin@example:/config/> edit container system
    admin@example:/config/container/system/> set hostname sys101
    admin@example:/config/container/system/> leave
    admin@example:/> container shell system
    root@sys101:/#

In fact, the container `hostname` setting supports the same format
specifiers as the host's `hostname` setting:

 - `%i`: OS ID, from `/etc/os-release`, from Menuconfig branding
 - `%h`: Default hostname, from `/etc/os-release`, from branding
 - `%m`: NIC specific part of base MAC, e.g., to `c0-ff-ee`
 - `%%`: Literal %

The most useful combination is probably `"container-name-%m"`, which in
this example give the container hostname `container-name-c0-ff-ee`.

[^1]: this does not apply to the admin-exec command `container run`.
    This command is intended to be used for testing and evaluating
	container images.  Such containers are given a private network
	behind an IP masquerading bridge.


### Application Container: nftables

Infix currently does not have a native firewall configuration, and even
when it does it will never expose the full capabilities of `nftables`.
For advanced setups, the following is an interesting alternative.

    admin@example:/> configure
    admin@example:/config> edit container nftables
    admin@example:/config/container/nftables/> set image ghcr.io/kernelkit/curios-nftables:edge
    admin@example:/config/container/nftables/> set network host
    admin@example:/config/container/nftables/> set privileged
    admin@example:/config/container/nftables/> edit mount nftables.conf
    admin@example:/config/container/nftables/mount/nftables.conf/> set target /etc/nftables.conf
    admin@example:/config/container/nftables/mount/nftables.conf/> text-editor content
    ... interactive editor starts up where you can paste your rules ...
    admin@example:/config/container/nftables/mount/nftables.conf/> leave

Notice how we `set network host`, so the container can see and act on
all the host's interfaces, and that we also have to run the container
in *privileged* mode.


### Application Container: ntpd

The default NTP server/client in Infix is Chrony, a fully working and
capable workhorse for most use-cases.  However, it does not support a
feature like multicasting, for that you need ISC ntpd.

As we did with `nftables`, previously, we can use host networking and
set up a read-only config file that is bind-mounted into the container's
file system and store in the host's `startup-config`.  However, `ntpd`
also saves clock drift information in `/var/lib/ntpd`, so we will also
use volumes in this example.

    admin@example:/> configure
    admin@example:/config> edit container ntpd
    admin@example:/config/container/ntpd/> set image ghcr.io/kernelkit/curios-ntpd:edge
    admin@example:/config/container/ntpd/> set network interface ntpd    # From veth0 above
    admin@example:/config/container/ntpd/> edit mount ntp.conf
    admin@example:/config/container/ntpd/mount/ntp.conf/> set target /etc/ntp.conf
    admin@example:/config/container/ntpd/mount/ntp.conf/> text-editor content
    ... interactive editor starts up where you can paste your rules ...
    admin@example:/config/container/ntpd/mount/ntp.conf/> end
    admin@example:/config/container/ntpd/> edit volume varlib
    admin@example:/config/container/ntpd/volume/varlib/> set target /var/lib
    admin@example:/config/container/ntpd/volume/varlib/> leave
    admin@example:/> copy running-config startup-config

The `ntp.conf` file is stored in the host's `startup-config` and any
state data in the container's `/var/lib` is retained between reboots
and across image upgrades.


Advanced
--------

This section covers advanced, and sometimes dangerous, topics.  Please
read any warnings and always consider the security aspects.

### Running Host Commands From Container

SSH login with keys is very handy, both remote scripting friendly *and
secure*, but it does require a few extra configuration steps.  The way
to set it up is covered in part in [SSH Authorized Key][4].

Another *insecure* approach is to access the host system directly,
bypassing the namespaces that make up the boundary between host and
container.

> **Security:** Please note, this completely demolishes the isolation
> barrier between container and host operating system.  It is only
> suitable in situations where the container serves more as a unit of
> distribution rather than as a separate component of the system.
> *Strongly recommended* to use this only in trusted setups!  Consider
> also limiting the time frame in which this is active!

First, enable *Privileged* mode, this unlocks the door and allows the
container to manage resources on the host system.  An example is the
`nftables` container mentioned previously.

    admin@example:/config/container/system/> set privileged

Second, mount the host's `/proc/1` directory to somewhere inside your
container.  Here we pick `/1`:

	admin@example:/config/container/system/> edit mount host
	admin@example:/config/container/system/mount/host/> set source /proc/1
	admin@example:/config/container/system/mount/host/> set target /1
	admin@example:/config/container/system/mount/host/> leave

Third, from inside the container, use the host's PID 1 namespaces with
the `nsenter`[^2] command to slide through the container's walls.  Here
we show two example calls to `hostname`, first the container's own name
and then asking what the hostname is on the host:

	root@sys101:/# hostname
	sys101
    root@sys101:/# nsenter -m/1/ns/mnt -u/1/ns/uts -i/1/ns/ipc -n/1/ns/net hostname
	example

One use-case for this method is when extending Infix with a management
container that connects to other systems.  For some tips on how to
control an Infix system this way, see [Scripting Infix](scripting.md).

[^2]: The `nsenter` program is available from either the util-linux
    package in Debian/Ubuntu/Mint, or in BusyBox.  Note, however,
	it may not be enabled by default in BusyBox.


[0]:      networking.md
[1]:      https://github.com/kernelkit/infix/blob/main/src/confd/yang/infix-containers.yang
[2]:      https://github.com/troglobit/mg
[3]:      https://github.com/opencontainers/image-spec/blob/main/image-layout.md
[4]:      system.md#ssh-authorized-key
[5]:      https://docs.docker.com/build/exporters/oci-docker/
[6]:      https://man7.org/linux/man-pages/man7/capabilities.7.html
[podman]: https://podman.io
