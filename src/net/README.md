net - an iproute2 wrapper
=========================

The net tool is a wrapper around the iproute2 and ethtool programs to
set up networking on a Linux system.  It also handles transitions from
a previously configured state to the next while attempting to keep the
amount of changes needed to a minimum.


Overview
--------

net applies a configuration from `/run/net/next`, where `next` is a file
that holds the number of the current generation.  It is up to the user
to create the below structure with commands to run.  Consider the case
when `next` contains `0`, net reads all interfaces, in dependency order,
from `/run/net/0/` and runs all the `ip-link` and `ip-addr` commands.
When it is done, it writes `0` to the file `/run/net/gen` and removes
the `next` file to confirm the next generation has been activated.

Note: it is currently up to the user to remove any old generation.

     /run/net/0/
	   |-- lo/
	   |    |-- deps/
	   |    |-- ip-link
	   |    `-- ip-addr
	   |-- br0/
	   |    |-- deps/
	   |    |    |-- eth1 -> ../../eth1
	   |    |    |-- eth2 -> ../../eth2
	   |    |    |-- eth3 -> ../../eth3
	   |    |    `-- lag0 -> ../../lag0
	   |    |-- ip-link.up
	   |    `-- ip-addr.up
	   |-- eth0/
	   |    |-- deps/
	   |    |-- ip-link.up
	   |    `-- ip-addr.up
	   |-- lag0/
	   |    |-- deps/
	   |    |    |-- eth4 -> ../../eth4
	   |    |    `-- eth5 -> ../../eth5
	   |    |-- ip-link.up
	   |    `-- ip-addr.up
	   `-- vlan1/
	        |-- deps/
	        |    `-- br0 -> ../../br0
	        |-- ip-link.up
	        `-- ip-addr.up

The `deps/` sub-directory for each of the interfaces contains symlinks
to all interfaces that this interface depends on.  I.e., when bringing
networking up or down these dependent interfaces are evaluated in order
creating a dependency tree:

               vlan1
         _____/____
        [___br0____]
        /  /   \   \
    eth1 eth2 eth3 lag0
                   /  \
                eth4  eth5

Essentially, all leaves must be set up before their parents.


Concepts
--------

Conceptually, net is built to complement sysrepo. The idea is to listen
to added, deleted, modified states in sysrepo changes to a candidate
configuration and generate the `next` generation.


    /run/net/<NUM+1>/
	   |-- lo/
	   |    |-- deps/
	   |    |-- ip-link.up
	   |    `-- ip-addr.up
	   |-- br0/
	   |    |-- deps/
	   |    |    |-- eth1 -> ../../eth1
	   |    |    |-- eth2 -> ../../eth2
	   |    |    |-- eth3 -> ../../eth3
	   |    |    |-- eth4 -> ../../eth4
	   |    |    `-- lag0 -> ../../lag0
	   |    `-- ip-link.up
	   |-- eth0/
	   |    |-- deps/
	   |    |-- ip-link.up
	   |    `-- ip-addr.up
	   |-- eth4/
	   |    |-- deps/
	   |    |-- ip-link.dn
	   |    `-- ip-link.up
	   |-- lag0/
	   |    |-- deps/
	   |    |    `-- eth5 -> ../../eth5
	   |    `-- ip-link.up
	   `-- vlan1/
	        |-- deps/
	        |    `-- br0 -> ../../br0
	        |-- ip-link.up
	        `-- ip-addr.up


Interfaces can be evaluated in any order.  The `deps/` directory of each
is interface is always evaluated first.  For each dependency, `foo.dn`
is evaluated first and `foo.up` is evaluated last.  Any `ip-link` script
is also evaluated before any `ip-addr` script.

In the case above, interface `eth4` has been moved from `lag0` to `br0`,
so we need to run `eth4/ip-link.dn` to remove `eth4` from `lag0` before
its `ip-link.up` script sets `eth4` as a bridge member instead.
