#!/bin/sh

set -e

common=$(dirname $(readlink -f "$0"))

cd $BINARIES_DIR
cp $common/rootfs.its .

mkimage -E -p 0x1000 -B 0x1000 -k $2 -f rootfs.its rootfs.itb

cp rootfs.itb rootfs.itbh
truncate -s $((0x1000)) rootfs.itbh
