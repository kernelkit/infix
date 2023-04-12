net - an iproute2 wrapper
=========================

The net tool is a wrapper around the iproute2 and ethtool programs to
set up networking on a Linux system.  It also handles transitions from
a previously configured state to the next while attempting to keep the
amount of changes needed to a minimum.


Concept Overview
----------------

net applies a configuration from `/run/net/next`, where `next` is a file
that holds the number of the next net generation.  It is up to the user,
i.e., sysrepo, to create the below tree structure with commands to run.

The setup we start with and later move `eth4` from `lag0` to `br0`.

               vlan1
         ______/____
        [____br0____]
        /  /   \    \
    eth1 eth2 eth3  lag0
                    /  \
                 eth4  eth5

Consider the case when `next` contains `0`, net reads all interfaces, in
dependency order, from `/run/net/0/` running all `cmd.init` scripts.
When done, it writes `0` to the file `/run/net/gen` and then removes the
`next` file to confirm the generation has been activated.

**Note:** it is currently up to the user to remove any old generation.

     /run/net/0/
	   |-- br0/
	   |    |-- deps/
	   |    |    |-- eth1 -> ../../eth1
	   |    |    |-- eth2 -> ../../eth2
	   |    |    |-- eth3 -> ../../eth3
	   |    |    `-- lag0 -> ../../lag0
	   |    `-- ip.init
	   |-- eth0/
	   |    |-- deps/
	   |    `-- ip.init
	   |-- ethX/
	   |    |-- ...
	   :    :
	   |-- lag0/
	   |    |-- deps/
	   |    |    |-- eth4 -> ../../eth4
	   |    |    `-- eth5 -> ../../eth5
	   |    `-- ip.init
	   `-- vlan1/
	        |-- deps/
	        |    `-- br0 -> ../../br0
	        `-- ip.init

The `deps/` sub-directory for each of the interfaces contains symlinks
to any interfaces this interface may depend on.  I.e., those interfaces
are evaluated first.

Essentially, all leaves must be set up before their parents.  Moving a
leaf from one parent to another, e.g., from lag0 to br0, is tricky, it
involves traversing the previous dependency order when removing leaves,
and traversing the next dependency order when adding, see next section
for an example.


Example
-------

net is built to complement sysrepo.  The idea is to listen to any added,
deleted, modified states in sysrepo changes to a candidate configuration
and generate the `next` generation.  However, as mentioned previously,
when moving leaves between parents we must do so in the dependency order
of the current generation.

So, the user (sysrepo) needs to add `cmd.exit` scripts in the current
tree for every leaf that leaves a parent, and `cmd.init` scripts for
leaves that are new or added to parents in the next generation.

In our example, interface `eth4` is moved from `lag0` to `br0`, so we
need to run `eth4/ip.exit` in the current generation first to remove
`eth4` from `lag0` before its `ip.init` script in the next generation
sets `eth4` as a bridge member instead.

We traverse the current generation and execute all `cmd.exit` scripts:

    /run/net/<GEN>/
	   |-- br0/
	   |    |-- deps/
	   |    |    |-- eth1 -> ../../eth1
	   |    |    |-- eth2 -> ../../eth2
	   |    |    |-- eth3 -> ../../eth3
	   |    |    `-- lag0 -> ../../lag0
	   |    `-- ip.init
	   |-- eth0/
	   |    |-- deps/
	   |    `-- ip.init
	   |-- eth4/
	   |    |-- deps/
	   |    |-- ip.exit
	   |    `-- ip.init
	   |-- lag0/
	   |    |-- deps/
	   |    |    |-- eth4 -> ../../eth4
	   |    |    `-- eth5 -> ../../eth5
	   |    `-- ip.init
	   `-- vlan1/
	        |-- deps/
	        |    `-- br0 -> ../../br0
	        `-- ip.init

Now we can run all the `cmd.init` scripts in the next generation:

    /run/net/<GEN+1>/
	   |-- br0/
	   |    |-- deps/
	   |    |    |-- eth1 -> ../../eth1
	   |    |    |-- eth2 -> ../../eth2
	   |    |    |-- eth3 -> ../../eth3
	   |    |    |-- eth4 -> ../../eth4
	   |    |    `-- lag0 -> ../../lag0
	   |    `-- ip.init
	   |-- eth0/
	   |    |-- deps/
	   |    `-- ip.init
	   |-- eth4/
	   |    |-- deps/
	   |    `-- ip.init
	   |-- lag0/
	   |    |-- deps/
	   |    |    `-- eth5 -> ../../eth5
	   |    `-- ip.init
	   `-- vlan1/
	        |-- deps/
	        |    `-- br0 -> ../../br0
	        `-- ip.init

When there are no changes compared to the previous generation, the
`cmd.init` scripts can be empty.  The existence of an `ip.init` script,
however, means that it is allowed to be brought up on `net up`.  The
existence of a directory (named after an interface) means the interface
should be brought down on `net down`.  Doing `net up ifname` is a subset
of that for a single interface `ifname`.
