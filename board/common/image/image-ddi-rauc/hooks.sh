#!/bin/sh

set -e

note()
{
    echo "$*" >&2
}

die()
{
    echo "ERROR: $*" >&2
    exit 1
}

case "$1" in
    slot-pre-install)
        [ "$RAUC_SLOT_CLASS" = "rootfs" ] || break

	dst=/dev/mapper/"$RAUC_SLOT_BOOTNAME" \
	    || die "$RAUC_SLOT_BOOTNAME is not available"
	dstsize=$(lvs --reportformat=json --units b "$dst" \
		    | jq -r .report[0].lv[0].lv_size \
		    | tr -d B)

	src="$RAUC_BUNDLE_MOUNT_POINT"/ddi.img
	[ -f "$src" ] \
	    || die "No DDI in bundle"
	srcsize=$(stat -L -c %s "$src")

	kpartx -d "$dst" \
	    || die "Unable to tear down partitions"

	[ "$dstsize" -eq "$srcsize" ] || {
	    lvm lvresize --yes --size ${newsize}B "$dst" \
		|| die "Unable to resize $RAUC_SLOT_BOOTNAME to ${srcsize}B"
	}

	note "$RAUC_SLOT_BOOTNAME is ready to accept update"
	;;
    *)
        exit 1
        ;;
esac
