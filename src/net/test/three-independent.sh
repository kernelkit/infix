#!/bin/sh
# - Verify correct bring up of three independent interfaces
# - Verify removal of one
# - Verify net down
# - Verify net up <one>
TEST_DIR=$(dirname "$0")
. "$TEST_DIR/lib.sh"

say "Verify bringup of generation $gen"
create_iface eth0 10.0.0.1/24
create_iface eth1 10.0.1.1/24
create_iface eth2 10.0.2.1/24

netdo

assert_iface eth0 10.0.0.1/24
assert_iface eth1 10.0.1.1/24
assert_iface eth2 10.0.2.1/24

sep
say "Verify removal of an interface"
remove_iface eth1
init_next_gen eth0 eth2

netdo

assert_iface eth0 10.0.0.1/24
assert_noiface eth1
assert_iface eth2 10.0.2.1/24

sep
say "Verify net down"
netdown
assert_iface_flag "Verify eth0 DOWN" eth0 UP false
assert_iface_flag "Verify eth2 DOWN" eth2 UP false

sep
say "Verify net up eth2"
netup eth2
assert_iface_flag "Verify eth0 DOWN" eth0 UP false
assert_iface_flag "Verify eth2 UP"   eth2 UP true

