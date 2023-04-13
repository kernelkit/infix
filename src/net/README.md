net - an iproute2 wrapper
=========================

The net tool is a wrapper around the iproute2 and ethtool programs to
set up networking on a Linux system.  It also handles transitions from
a previously configured state to the next while attempting to keep the
amount of changes needed to a minimum.


Concept Overview
----------------

net applies a configuration from `/run/net/next`, where `next` is a file
that holds the number of the current generation.  It is up to the user,
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
dependency order, from `/run/net/0/` running all `ip-link` and `ip-addr`
commands.  When done, it writes `0` to the file `/run/net/gen` and then
removes the `next` file to confirm the generation has been activated.

**Note:** it is currently up to the user to remove any old generation.

     /run/net/0/
	   |-- lo/
	   |    |-- deps/
	   |    |-- ip-link.up
	   |    `-- ip-addr.up
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
	   |-- ethX/
	   |    |-- ...
	   :    :
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
to any interfaces this interface may depend on.  I.e., those interfaces
are evaluated first.

Essentially, all leaves must be set up before their parents.  Moving a
leaf from one parent to another, e.g., from lag0 to br0, is tricky, it
involves traversing the previous dependency order when removing leaves,
and traversing the next dependency order when addning, see next section
for an example.


Example
-------

net is built to complement sysrepo.  The idea is to listen to any added,
deleted, modified states in sysrepo changes to a candidate configuration
and generate the `next` generation.  However, as mentioned previously,
when moving leaves between parents we must do so in the dependency order
of the current generation.

So, the user (sysrepo) needs to add `.dn` scripts in the current tree,
and `.up` scripts in the next.

In our example, interface `eth4` is moved from `lag0` to `br0`, so we
need to run `eth4/ip-link.dn` in the current generation first to remove
`eth4` from `lag0` before its `ip-link.up` script in the next generation
sets `eth4` as a bridge member instead.

We traverse the current generation and execute all `.dn` scripts:

    /run/net/<GEN>/
	   |-- lo/
	   |    `-- deps/
	   |-- br0/
	   |    |-- deps/
	   |    |    |-- eth1 -> ../../eth1
	   |    |    |-- eth2 -> ../../eth2
	   |    |    |-- eth3 -> ../../eth3
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
	   |    |    |-- eth4 -> ../../eth4
	   |    |    `-- eth5 -> ../../eth5
	   |    `-- ip-link.up
	   `-- vlan1/
	        |-- deps/
	        |    `-- br0 -> ../../br0
	        |-- ip-link.up
	        `-- ip-addr.up

Now we can run all the `.up` scripts in the next generation:

    /run/net/<GEN+1>/
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


