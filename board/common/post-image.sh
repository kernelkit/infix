#!/bin/sh

common=$(dirname "$(readlink -f "$0")")
. $common/lib.sh

load_cfg BR2_EXTERNAL_INFIX_PATH

load_cfg BR2_ARCH
load_cfg SIGN_KEY

ixmsg "Signing SquashFS Image"
$common/sign.sh $BR2_ARCH $SIGN_KEY

ixmsg "Creating RAUC Update Bundle"
$common/mkrauc.sh $BR2_ARCH $SIGN_KEY

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
    load_cfg BR2_DEFCONFIG
    ixmsg "Creating GNS3 Appliance, $GNS3_APPLIANCE_RAM MiB with $GNS3_APPLIANCE_IFNUM ports"
    $common/mkgns3a.sh $(basename $BR2_DEFCONFIG _defconfig | tr _ -) $GNS3_APPLIANCE_RAM $GNS3_APPLIANCE_IFNUM
fi

load_cfg FIT_IMAGE
if [ "$FIT_IMAGE" = "y" ]; then
    ixmsg "Creating Traditional FIT Image"
    $common/mkfit.sh
fi
