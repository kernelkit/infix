#!/bin/sh
# - Basic bridge with three bridge ports
# - Add another port
# - Remove a port

TEST_DIR=$(dirname "$0")
. "$TEST_DIR/lib.sh"

################################################
say "Verify bringup of basic bridge with three ports"
create_iface eth0
create_iface eth1
create_iface eth2
create_bridge br0 "" eth0 eth1 eth2

netdo

assert_bridge_ports br0 true eth0 eth1 eth2
assert_iface br0

################################################
sep
say "Verify add another bridge port (eth3)"
init_next_gen
create_iface_data br0

create_iface eth3
add_brport br0 eth3

netdo

assert_bridge_ports br0 true eth0 eth1 eth2 eth3
assert_iface br0

################################################
sep
say "Verify delete a bridge port (eth1)"
del_brport br0 eth1

init_next_gen
create_iface_data br0
create_iface_data eth1

netdo

assert_bridge_ports br0 true eth0 eth2 eth3
assert_bridge_ports br0 false eth1
assert_iface br0
