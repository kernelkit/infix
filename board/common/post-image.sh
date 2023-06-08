#!/bin/sh
common=$(dirname "$(readlink -f "$0")")
. $common/lib.sh

load_cfg BR2_DEFCONFIG
load_cfg BR2_EXTERNAL_INFIX_PATH
NAME=infix-$(basename "$BR2_DEFCONFIG" _defconfig | tr _ - | sed 's/x86-64/x86_64/')

ver()
{
    [ -n "$RELEASE" ] || return
    GITVER=$(git describe --tags --dirty)
    if echo "$GITVER" |grep -q 'latest-'; then
	latest=$(echo "$GITVER" | sed 's/latest-[0-9]*-\(g.*\)/\1/')
	cd "$BR2_EXTERNAL_INFIX_PATH/buildroot" || exit 1
	br2ver=$(git describe --tags --dirty | sed 's/..\([^-]*\).*/\1/g')
	cd - || exit 1
	GITVER=$br2ver-$latest
    fi
    printf "-%s" "$GITVER"
}

load_cfg SIGN_ENABLED
if [ "$SIGN_ENABLED" = "y" ]; then
    load_cfg BR2_ARCH
    load_cfg SIGN_KEY

    ixmsg "Signing SquashFS Image"
    $common/sign.sh $BR2_ARCH $SIGN_KEY

    ixmsg "Creating RAUC Update Bundle"
    $common/mkrauc.sh $NAME $BR2_ARCH $SIGN_KEY
fi

load_cfg DISK_IMAGE
load_cfg DISK_IMAGE_SIZE
if [ "$DISK_IMAGE" = "y" ]; then
    ixmsg "Creating Disk Image"
    $common/mkdisk.sh -a $BR2_ARCH -s $DISK_IMAGE_SIZE
fi

load_cfg GNS3_APPLIANCE
if [ "$GNS3_APPLIANCE" = "y" ]; then
    load_cfg GNS3_APPLIANCE_RAM
    load_cfg GNS3_APPLIANCE_IFNUM
    ixmsg "Creating GNS3 Appliance, $GNS3_APPLIANCE_RAM MiB with $GNS3_APPLIANCE_IFNUM ports"
    $common/mkgns3a.sh $NAME $GNS3_APPLIANCE_RAM $GNS3_APPLIANCE_IFNUM
fi

load_cfg FIT_IMAGE
if [ "$FIT_IMAGE" = "y" ]; then
    ixmsg "Creating Traditional FIT Image"
    $common/mkfit.sh
fi

# For use outside of the build system, e.g., Qeneth
ln -sf rootfs.squashfs "$BINARIES_DIR/$NAME$(ver).img"
