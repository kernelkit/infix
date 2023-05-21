Containers in Infix
===================

The default builds of Infix do not enable any container support.  This
because it is not a common customer feature, and it extends build times
due to bringing in a build-time dependency on Go.

However,customer specific builds may have it, and you can also roll your
own based on any of the available `defconfigs`.  For example:

    cd infix/
    make x86_64_defconfig


Docker Containers with Podman
-----------------------------

Run menuconfig, search for `podman`, enable that and build:

	make menuconfig
	...
	make

Enabling [podman][] select `crun`, `conmon`, and all other dependencies.
The build will take a while, but eventually you can:

    make run

> A convenience alias `docker=podman` is available, but remember, not
> all features or syntax of docker is available in podman.

Test it out with an example:

    podman run -it --rm docker://hello-world

or a little web server:

    podman run -d --rm -p 80:80 docker://nginx:alpine

In detached (`-d`) state you can check the status using `podman ps` or
try to connect to the web server:

    curl http://localhost

or

    lynx http://localhost


### Multiple Networks

It is also possible to start a container with multiple networks.  The
approach shown here uses CNI profiles, which means the interfaces names
inside the container will always be: `eth0`, `eth1`, etc.

Pending VETH support in Infix/NETCONF, the following example sets up a
VETH pair, then creates a CNI profile, and finally starts a container
with two profiles, the default podman bridge and the VETH profile:

     ip link add local1 type veth peer local1_peer
     ip link set local1 up
     ip addr add 192.168.0.1/24 dev local1
     cni create host net1 local1_peer 192.168.0.42/24
     podman run -d --rm --net=podman,net1 --privileged docker://troglobit/buildroot:latest

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


### NETCONF Build

If you've followed this tutorial then you now have a NETCONF based Infix
system running.  To run containers on it you need to leverage the Hybrid
mode, described in the README but also repeated below.

To start containers in *Hybrid Mode*, provided the images have been
downloaded with `podman pull` first):

```
root@infix:/cfg/start.d$ cat <<EOF >20-enable-container.sh
#!/bin/sh
podman-service -e -d "Nginx container" -p "-p 80:80" nginx:alpine
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

[podman]: https://podman.io
