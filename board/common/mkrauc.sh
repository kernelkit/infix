#!/bin/sh

set -e

GIT_VERSION=$(git -C $BR2_EXTERNAL_INFIX_PATH describe --always --dirty --tags)
if [ -n "$INFIX_RELEASE" ]; then
    rel="-${INFIX_RELEASE}"
fi

name=$1${rel}
arch=$2
sign=$3

crt=$(ls $sign/*.crt)
key=$(ls $sign/*.key)

common=$(dirname $(readlink -f "$0"))

work=$BUILD_DIR/mkrauc
mkdir -p $work

cp -f $common/rauc-hooks.sh $work/hooks.sh

# RAUC internally uses the file extension to find a suitable install
# handler, hence the name must be .img
cp -f $BINARIES_DIR/rootfs.squashfs $work/rootfs.img
cp -f $BINARIES_DIR/rootfs.itbh $work/rootfs.itbh

cat >$work/manifest.raucm <<EOF
[update]
compatible=infix-${arch}
version=${GIT_VERSION}

[bundle]
format=verity

[hooks]
filename=hooks.sh

[image.rootfs]
filename=rootfs.img
hooks=post-install
EOF

rm -f $BINARIES_DIR/$name.pkg

rauc --cert=$crt --key=$key \
    bundle $work $BINARIES_DIR/$name.pkg

# Bootstrap a RAUC status file showing the newly created image
# installed to both the primary and secondary slots. This then bundled
# in the aux partition in mkdisk.sh, so that RAUC (on the target) can
# always report the installed versions.
rauc info --no-verify --output-format=shell $BINARIES_DIR/$name.pkg >$work/rauc.info
. $work/rauc.info
tstamp=$(date -u +%FT%TZ)
cat >$work/rauc.status <<EOF
[slot.rootfs.0]
bundle.compatible=$RAUC_MF_COMPATIBLE
bundle.version=$RAUC_MF_VERSION
status=ok
sha256=$RAUC_IMAGE_DIGEST_0
size=$RAUC_IMAGE_SIZE_0
installed.timestamp=$tstamp
installed.count=1
activated.timestamp=$tstamp
activated.count=1

[slot.rootfs.1]
bundle.compatible=$RAUC_MF_COMPATIBLE
bundle.version=$RAUC_MF_VERSION
status=ok
sha256=$RAUC_IMAGE_DIGEST_0
size=$RAUC_IMAGE_SIZE_0
installed.timestamp=$tstamp
installed.count=1
activated.timestamp=$tstamp
activated.count=1
EOF
