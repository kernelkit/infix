#!/bin/sh
# - Verify correct bring up of three independent interfaces
# - Verify removal of one
# - Verify net down
# - Verify net up <one>
. "./lib.sh"

create_iface()
{
    ifname=$1
    address=$2

    mkdir -p "$NET_DIR/$gen/$ifname/deps"
    cat <<-EOF >"$NET_DIR/$gen/$ifname/init.ip"
	link add $ifname type dummy
	addr add $address dev $ifname
	link set $ifname up
EOF
    echo "up" > "$NET_DIR/$gen/$ifname/admin-state"
}

check_iface()
{
    ifname=$1
    address=$2
    state=$(tr '[:lower:]' '[:upper:]' < "$NET_DIR/$gen/$ifname/admin-state")

    addr=$(ip -br -j addr show "$ifname" | jq -r '.[] | .addr_info[0].local')
    plen=$(ip -br -j addr show "$ifname" | jq -r '.[] | .addr_info[0].prefixlen')
    addr="$addr/$plen"
    updn=$(ip -br -j link show "$ifname" | jq -r '.[] | .flags[] | select(index("UP"))' | head -1)

#    echo "$state => $ifname: $updn $addr"
    assert "Verify $ifname state $state"      "$state"   = "$updn"
    assert "Verify $ifname address $address"  "$address" = "$addr"
}

say "Verify bringup of generation $gen"
create_iface eth0 10.0.0.1/24
create_iface eth1 10.0.1.1/24
create_iface eth2 10.0.2.1/24

netdo

check_iface eth0 10.0.0.1/24
check_iface eth1 10.0.1.1/24
check_iface eth2 10.0.2.1/24

