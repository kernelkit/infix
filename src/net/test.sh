#!/bin/sh
# Generate test directory structure for verifying the net tool
set -e

NET_DIR=/tmp/net
export NET_DIR

gensh()
{
    cat <<-EOF >"$1"
	#!/bin/sh
	echo "Running \$0 ..."
	EOF
    chmod +x "$1"
}

check()
{
    if ./net; then
	printf "\n[\033[1;32m OK \033[0m] Checking %s\n" "$1"
	return 0
    fi

    printf "\n[\033[1;31FAIL\033[0m] Checking %s\n" "$1"
    return 1
}

echo "Verify that all leaves and dependencies are"
echo "evaluated first:"
echo "                     vlan1"
echo "               ______/____"
echo "              [____br0____]"
echo "              /  /   \\    \\"
echo "lo  eth0  eth1 eth2 eth3  lag0"
echo "                          /  \\"
echo "                       eth4  eth5"
printf "_______________________________________________\n\n"

mkdir -p $NET_DIR/0
echo 0 > $NET_DIR/next

mkdir -p $NET_DIR/0/lo/deps
gensh $NET_DIR/0/lo/ip.init

mkdir -p $NET_DIR/0/eth0/deps
mkdir -p $NET_DIR/0/eth1/deps
mkdir -p $NET_DIR/0/eth2/deps
mkdir -p $NET_DIR/0/eth3/deps
mkdir -p $NET_DIR/0/eth4/deps

gensh $NET_DIR/0/eth0/ip.init
gensh $NET_DIR/0/eth1/ip.init
gensh $NET_DIR/0/eth2/ip.init
gensh $NET_DIR/0/eth3/ip.init
gensh $NET_DIR/0/eth4/ip.init

mkdir -p $NET_DIR/0/lag0/deps
ln -sf ../../eth3 $NET_DIR/0/lag0/deps/
ln -sf ../../eth4 $NET_DIR/0/lag0/deps/
gensh $NET_DIR/0/lag0/ip.init

mkdir -p $NET_DIR/0/br0/deps
ln -sf ../../eth1 $NET_DIR/0/br0/deps/
ln -sf ../../eth2 $NET_DIR/0/br0/deps/
ln -sf ../../lag0 $NET_DIR/0/br0/deps/
gensh $NET_DIR/0/br0/ip.init

mkdir -p $NET_DIR/0/vlan1/deps
ln -sf ../../br0 $NET_DIR/0/vlan1/deps/
gensh $NET_DIR/0/vlan1/ip.init

check "initial startup, gen 0"

printf "_______________________________________________\n\n"
mkdir -p $NET_DIR/1

mkdir -p $NET_DIR/1/lo/deps
mkdir -p $NET_DIR/1/eth0/deps
mkdir -p $NET_DIR/1/eth1/deps
mkdir -p $NET_DIR/1/eth2/deps
mkdir -p $NET_DIR/1/eth3/deps
mkdir -p $NET_DIR/1/eth4/deps

mkdir -p $NET_DIR/1/lag0/deps
ln -sf ../../eth4 $NET_DIR/1/lag0/deps/

mkdir -p $NET_DIR/1/br0/deps
ln -sf ../../eth1 $NET_DIR/1/br0/deps/
ln -sf ../../eth2 $NET_DIR/1/br0/deps/
ln -sf ../../eth3 $NET_DIR/1/br0/deps/

mkdir -p $NET_DIR/1/vlan1/deps
ln -sf ../../br0 $NET_DIR/1/vlan1/deps/

gensh $NET_DIR/0/eth3/ip.exit
gensh $NET_DIR/1/eth3/ip.init
echo 1 > $NET_DIR/next

check "move eth3 from lag0 to br0"

exit 0
