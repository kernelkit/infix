#!/bin/sh
# - Verify correct bring up of three independent interfaces
# - Verify removal of one
# - Verify net down
# - Verify net up <one>

NET_DIR=/tmp/net
export NET_DIR

gen=-1

init()
{
    ip link set lo up
}

netdo()
{
    ../net -vd apply
}

create_ng()
{
    _=$((gen += 1))
    mkdir -p "$NET_DIR/$gen"
    echo $gen > "$NET_DIR/next"
}

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

    echo "$state => $ifname: $updn $addr"

    if [ "$state" != "$updn" ]; then
	echo "Failed to bring $ifname $state ($updn)"
	exit 1
    fi
    if [ "$address" != "$addr" ]; then
	echo "Failed to set $ifname $address ($addr)"
	exit 1
    fi
}

init

create_ng
create_iface eth0 10.0.0.1/24
create_iface eth1 10.0.1.1/24
create_iface eth2 10.0.2.1/24
tree "$NET_DIR/"
netdo

check_iface eth0 10.0.0.1/24
check_iface eth1 10.0.1.1/24
check_iface eth2 10.0.2.1/24

ip link
ip addr


tree "$NET_DIR/"
