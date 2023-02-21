#!/bin/sh

set -e

common=$(dirname $(readlink -f "$0"))
root=$BUILD_DIR/genimage.root
tmp=$BUILD_DIR/genimage.tmp


mkdir -p $root/aux
cp -f $BINARIES_DIR/uboot-env.bin $root/aux/uboot.env

rm -rf $tmp

genimage \
    --rootpath $root \
    --tmppath  $tmp \
    --inputpath $BINARIES_DIR \
    --outputpath $BINARIES_DIR \
    --config $common/genimage.cfg
