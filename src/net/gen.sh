#!/bin/sh
# Generate test directory structure for verifying the net tool

gensh()
{
    cat <<-EOF >"$1"
	#!/bin/sh
	echo "Running \$0 ..."
	EOF
    chmod +x "$1"
}

mkdir -p /tmp/net/1
echo 1 > /tmp/net/next

mkdir -p /tmp/net/1/lo/deps
mkdir -p /tmp/net/1/eth0/deps
mkdir -p /tmp/net/1/eth1/deps
mkdir -p /tmp/net/1/eth2/deps

mkdir -p /tmp/net/1/br0/deps
ln -sf ../../eth1 /tmp/net/1/br0/deps/
ln -sf ../../eth2 /tmp/net/1/br0/deps/

mkdir -p /tmp/net/1/vlan1/deps
ln -sf ../../br0 /tmp/net/1/vlan1/deps/

gensh /tmp/net/1/lo/ip-link.up
gensh /tmp/net/1/eth0/ip-link.up
gensh /tmp/net/1/eth1/ip-link.up
gensh /tmp/net/1/eth2/ip-link.up
gensh /tmp/net/1/br0/ip-link.up
gensh /tmp/net/1/vlan1/ip-link.up
