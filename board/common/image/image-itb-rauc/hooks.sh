#!/bin/sh

set -e

case "$1" in
        slot-post-install)
                [ "$RAUC_SLOT_CLASS" = "rootfs" ] || break

		echo "Updating signature information for $RAUC_SLOT_BOOTNAME"

		itbh=$(dirname $RAUC_IMAGE_NAME)/rootfs.itbh
		cp $itbh /mnt/aux/$RAUC_SLOT_BOOTNAME.itbh
		sync
                ;;
        *)
                exit 1
                ;;
esac
