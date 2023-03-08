#!/bin/sh
# Bridge qtap0 and eth2 in qbr0, enabling forwarding of LLDP frames in
# the group forward mask

TAP=qtap0
ETH=eth2
BR=qbr0
ADDR=192.168.0.1/24

if [ -n "$1" ]; then
    ETH=$1
fi

if [ -n "$2" ]; then
    ADDR=$2
fi

# https://interestingtraffic.nl/2017/11/21/an-oddly-specific-post-about-group_fwd_mask/
ip link add qbr0 type bridge group_fwd_mask 0x4000 2>/dev/null
ip link set "$TAP" master "$BR"
ip link set "$ETH" master "$BR"

for iface in $ETH $TAP $BR; do
    ip link set "$iface" up
done

ip addr flush "$TAP"
ip addr flush "$ETH"
ip addr add "$ADDR" dev "$BR" 2>/dev/null

echo "=================="
ip -br link
echo "=================="
ip -br addr
echo "=================="
bridge link
echo "=================="
