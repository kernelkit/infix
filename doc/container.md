Containers in Infix
===================

Default builds of Infix do not enable any container support.  See below
section, [Enabling Container Support](#enabling-container-support), for
details on how to enable it using Podman.

Networking in containers is provided by both Infix and the Container
Network Interface ([CNI](https://www.cni.dev/)) that Podman supports.


Docker Containers with Podman
-----------------------------

We assume you've booted into Infix and start with a familiar example:

    podman run -it --rm docker://hello-world

or a little web server:

    podman run -d --rm -p 80:80 docker://nginx:alpine

In detached (`-d`) state you can check the status using `podman ps` or
try to connect to the web server:

    curl http://localhost

or

    lynx http://localhost

> A convenience alias `docker=podman` is available, but remember, not
> all features or syntax of docker is available in podman.


### Multiple Networks

It is also possible to start a container with multiple networks.  The
approach shown here uses CNI profiles, which means the interfaces names
inside the container will always be: `eth0`, `eth1`, etc.

A common setup is to use a VETH pair, with one end in the container and
the other end routed, or bridged, to the rest of the world.  The Infix
[CLI Guide](cli.md) provides examples of both.  In either case you need
to create a matching CNI profile for one end of the VETH pair before
starting the container, here we use two network profiles, the default
podman bridge and the VETH profile:

     cni create host net1 veth0b 192.168.0.42/24
     podman run -d --rm --net=podman,net1 --entrypoint "/linuxrc" \
             --privileged docker://troglobit/buildroot:latest

The first profile (`podman`) is a the default bridged profile.  When a
container is started with that (default behavior), podman dynamically
creates a VETH pair which has one end attached as a bridge port in the
`cni-podman0` bridge managed by podman, and the other end is brought up
as `eth0` inside the container.

The second profile is the one we created, it uses the `host-device`
profile and does not create anything, it simply lifts the peer end of
the pair into the container as `eth1`.  This CNI profile can also be
used to hand over control of physical ports to a container.

> **Note:** here we start the container in `--privileged` mode.  This
> allows the container guest unfiltered access to the host system and it
> might not be what you want for a production system.  For that at least
> SECCOMP is recommended, which is out of scope for this tutorial.


### Hybrid Mode

If you've followed this tutorial then you now have a NETCONF based Infix
system running.  To run containers on it you need to leverage the Hybrid
mode, described in the README but also repeated below.

To start containers in *Hybrid Mode*, provided the images have been
downloaded with `podman pull docker://troglobit/buildroot:latest`):

```
root@infix:/cfg/start.d$ cat <<EOF >20-enable-container.sh
#!/bin/sh
# Remember to create the veth0a <--> vet0b pair in the CLI first!
cni create host net1 veth0b 192.168.0.42/24
podman-service -e -d "System container" -p "--net=podman,net1 -p 22:22 --entrypoint='/linuxrc' --privileged" buildroot:latest
exit 0
EOF
root@infix:/cfg/start.d$ chmod +x 20-enable-container.sh
```

Reboot to activate the changes.  To activate the changes without
rebooting, run the script and call `initctl reload`.

> **Note:** the `/etc` directory is a `tmpfs` ramdisk and contents will
> be lost on reboot, so to retain custom CNI profiles after reboot you
> need to either save them and restore in the script above, or recreate
> them on every boot.


Enabling Container Support
--------------------------

Container support is not enabled by default because it is not a common
customer feature, it also prolongs build times a lot due to bringing in
a build-time dependency on Go.

However, customer specific builds may have it, and you can also roll
your own based on any of the available `defconfigs`.  For example:

    cd infix/
    make x86_64_defconfig

Run menuconfig, search for `podman` using `/`, enable it and build:

	make menuconfig
	...
	make

Enabling [podman][] select `crun`, `conmon`, and all other dependencies.
The build will take a while, but eventually you can:

    make run

[podman]: https://podman.io
