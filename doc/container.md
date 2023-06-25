Containers in Infix
===================

* [Introduction](#introduction)
* [Docker Containers with Podman](#docker-containers-with-podman)
  * [Multiple Networks](#multiple-networks)
  * [Hybrid Mode](#hybrid-mode)
* [Enabling Containers](#enabling-containers)
* [Debugging Containers](#debugging-containers)


Introduction
------------

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


Debugging Containers
--------------------

If the host system is not powered down or rebooted properly, containers
may not start up as they should on the following boot.  Below is a very
common problem and solution shown.

```
root@infix-12-34-56:~$ podman ps
CONTAINER ID  IMAGE       COMMAND     CREATED     STATUS      PORTS       NAMES
root@infix-12-34-56:~$ grep nginx /var/log/syslog
Jun 25 10:15:48 infix-12-34-56 finit[1]: Service pod:nginx[2376] died, restarting in 5000 msec (10/10)
Jun 25 10:15:48 infix-12-34-56 finit[1]: Starting pod:nginx[2408]
Jun 25 10:15:53 infix-12-34-56 finit[1]: Service pod:nginx keeps crashing, not restarting.
```

If this the system is isolated from remote network access, start by
verifying the image is downloaded:

```
root@infix-12-34-56:/cfg/start.d$ podman images
REPOSITORY                     TAG         IMAGE ID      CREATED      SIZE
docker.io/library/nginx        alpine      4937520ae206  10 days ago  43.2 MB
docker.io/troglobit/buildroot  latest      68faf6b20f1a  6 weeks ago  41.4 MB
```

OK, let's see what the `podman-service` step (above) created:

```
root@infix-12-34-56:/cfg/start.d$ initctl show pod-nginx.conf
service name:pod :nginx podman run --name nginx --rm  -p 80:80 nginx:alpine   -- Nginx container
```

Try starting the container manually.  Remember to add the `-d` flag to
emulate detached/background operation:

```
root@infix-12-34-56:/cfg/start.d$ podman run --name nginx --rm -d -p 8080:80 nginx:alpine
Error: creating container storage: the container name "nginx" is already in use by 9c73bd8d505b1585d241595bfadede361b87f6c1be9a5656253b5a4d73da57e0. You have to remove that container to be able to reuse that name: that name is already in use
```

Aha, a lingering image with the same name!  Where is it?

```
root@infix-12-34-56:/cfg/start.d$ podman ps --all
CONTAINER ID  IMAGE                                 COMMAND               CREATED            STATUS                        PORTS               NAMES
f3386ae9517f  docker.io/troglobit/buildroot:latest                        About an hour ago  Exited (0) About an hour ago                      ecstatic_panini
bf0c6178ea26  docker.io/troglobit/buildroot:latest                        About an hour ago  Exited (0) About an hour ago                      determined_brown
385155f479c0  docker.io/troglobit/buildroot:latest                        About an hour ago  Exited (0) About an hour ago                      vibrant_engelbart
99a1b3319d9e  docker.io/troglobit/buildroot:latest                        About an hour ago  Exited (0) About an hour ago                      dreamy_tesla
9c73bd8d505b  docker.io/library/nginx:alpine        nginx -g daemon o...  11 minutes ago     Created                       0.0.0.0:80->80/tcp  nginx
8a5290504ebc  docker.io/troglobit/buildroot:latest                        10 minutes ago     Created                                           mystifying_liskov
```

Oh, we have two lingering containers that were created but did not stop
correctly.  Let's remove them:

```
root@infix-12-34-56:/cfg/start.d$ docker rm -f 9c73bd8d505b
9c73bd8d505b
root@infix-12-34-56:/cfg/start.d$ docker rm -f 8a5290504ebc
8a5290504ebc
```

Now we can manually restart the (supervised) container:

```
root@infix-12-34-56:/cfg/start.d$ initctl restart pod:nginx
root@infix-12-34-56:/cfg/start.d$ initctl status pod:nginx
     Status : running
   Identity : pod:nginx
Description : Nginx container
     Origin : /etc/finit.d/enabled/pod-nginx.conf
    Command : podman run --name nginx --rm -p 80:80 nginx:alpine
   PID file : none
        PID : 2669
       User : root
      Group : root
     Uptime : 15 sec
   Restarts : 11 (0/10)
  Runlevels : [---234-----]
     Memory : 63.8M
     CGroup : /system/pod-nginx cpu 0 [100, max] mem [0, max]
              ├─ 2669 podman run --name nginx --rm -p 80:80 nginx:alpine
              └─ 2816 conmon --api-version 1 -c 44d24aa7e98b67ff811596984462b902af3b09a04b4f9bef86e11d246b8cc2ff -u 44d24aa7e98b67ff8

Jun 25 10:15:48 infix-12-34-56 finit[1]: Service pod:nginx[2376] died, restarting in 5000 msec (10/10)
Jun 25 10:15:48 infix-12-34-56 finit[1]: Starting pod:nginx[2408]
Jun 25 10:15:53 infix-12-34-56 finit[1]: Service pod:nginx keeps crashing, not restarting.
Jun 25 10:47:55 infix-12-34-56 finit[1]: Starting pod:nginx[2669]
```


[podman]: https://podman.io
