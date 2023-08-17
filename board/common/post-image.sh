#!/bin/sh
common=$(dirname "$(readlink -f "$0")")
. $common/lib.sh

load_cfg BR2_ARCH
load_cfg BR2_DEFCONFIG
load_cfg BR2_EXTERNAL_INFIX_PATH
load_cfg BR2_TARGET_ROOTFS
NAME=infix-$(basename "$BR2_DEFCONFIG" _defconfig | tr _ - | sed 's/x86-64/x86_64/')

ver()
{
    if [ -n "$INFIX_RELEASE" ]; then
	printf -- "-%s" "$INFIX_RELEASE"
	return
    fi
}

load_cfg SIGN_ENABLED
if [ "$SIGN_ENABLED" = "y" ]; then
    load_cfg BR2_ARCH
    load_cfg SIGN_KEY

    ixmsg "Signing SquashFS Image"
    $common/sign.sh $BR2_ARCH $SIGN_KEY

    ixmsg "Creating RAUC Update Bundle"
    $common/mkrauc.sh "$NAME$(ver)" $BR2_ARCH $SIGN_KEY
fi

load_cfg DISK_IMAGE
if [ "$DISK_IMAGE" = "y" ]; then
    ixmsg "Creating Disk Image"

    bootcfg=
    if [ "$DISK_IMAGE_BOOT_DATA" ]; then
	bootcfg="-b $DISK_IMAGE_BOOT_DATA -B $DISK_IMAGE_BOOT_OFFSET"
    fi

    if [ "$BR2_TARGET_ROOTFS_SQUASHFS" != "y" ] && \
       [ ! -f "$BINARIES_DIR/rootfs.squashfs" ]; then
	ixmsg "  Injecting $DISK_IMAGE_RELEASE_URL"
	archive="$BINARIES_DIR/$(basename $DISK_IMAGE_RELEASE_URL)"
	[ -f "$archive" ] || wget -O"$archive" "$DISK_IMAGE_RELEASE_URL"
	tar -xa --strip-components=1 -C "$BINARIES_DIR" -f "$archive"
    fi
    $common/mkrauc-status.sh "$BINARIES_DIR/${NAME}.pkg" >"$BINARIES_DIR/rauc.status"
    $common/mkdisk.sh -a $BR2_ARCH -s $DISK_IMAGE_SIZE $bootcfg
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

if [ "$BR2_TARGET_ROOTFS_SQUASHFS" = "y" ]; then
    if [ -z "${NAME##*minimal*}" ]; then
	NAME=$(echo "$NAME" | sed 's/-minimal//')
    fi

    rel=$(ver)
    ln -sf rootfs.squashfs "$BINARIES_DIR/${NAME}${rel}.img"
    if [ -n "$rel" ]; then
	ln -sf "$BINARIES_DIR/${NAME}${rel}.img" "$BINARIES_DIR/${NAME}.img"
    fi
fi
