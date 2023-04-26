#!/bin/sh

set -e

K=10
M=20
G=30

size2int()
{
    # Use truncate's error message to convert input like "1K to "1024"
    # for us.
    #
    # - What do you mean by "fragile"?!
    truncate -s $1 /dev/null 2>&1 | awk '{ print($7); }'
}

dimension()
{
    if [ $total -ge $((4 << G)) ]; then
	bootsize=$(( 8 << M))
	auxsize=$((  8 << M))
	imgsize=$((  1 << G))
	cfgsize=$((512 << M))
	# var is at least ~1.5G
    elif [ $total -ge $((2 << G)) ]; then
	bootsize=$(( 8 << M))
	auxsize=$((  8 << M))
	imgsize=$((512 << M))
	cfgsize=$((256 << M))
	# var is at least ~1.75G
    elif [ $total -ge $((1 << G)) ]; then
	bootsize=$(( 4 << M))
	auxsize=$((  4 << M))
	imgsize=$((256 << M))
	cfgsize=$(( 64 << M))
	# var is at least ~0.5G
    elif [ $total -ge $((512 << M)) ]; then
	bootsize=$(( 4 << M))
	auxsize=$((  4 << M))
	imgsize=$((192 << M))
	cfgsize=$(( 16 << M))
	# var is at least ~100M
    else
	echo "Can't create disk images smaller than 512M"
	exit 1
    fi

    # Size var to fit whatever is left over. Also reserve another 32K
    # at the end to make room for the backup GPT.
    varsize=$(($total - $auxsize - 2 * $imgsize - $cfgsize))
    if [ "$bootoffs" ]; then
	varsize=$(($varsize - $bootsize))
    fi
    varsize=$(($varsize - (32 << K)))

    if [ "$bootoffs" ]; then
	# Align the end of the boot partition to an even MiB. E.g. if
	# boot was dimensioned to 4M, and bootoffs is 32K, then the
	# final bootsize becomes 4M - 32K, meaning aux will start on
	# exactly 4M.
	auxoffs=$bootsize
	bootsize=$(($bootsize - $bootoffs))
    else
	# No bootloader, place aux after GPT, resize it to end on an
	# even MiB (as is done for boot above).
	auxoffs=$((32 << K))
	auxsize=$(($auxsize - $auxoffs))
    fi
}

probeboot()
{
    # If we have built an EFI app, typically grub, make sure to
    # include it.
    if [ -d $BINARIES_DIR/efi-part/EFI ]; then
	bootoffs=$((32 << K))
    fi
}

genboot()
{
    if [ -d $BINARIES_DIR/efi-part/EFI ]; then
	bootimg=$(cat <<EOF
image efi-part.vfat {
	size = $bootsize
	vfat {
		file EFI {
			image = $BINARIES_DIR/efi-part/EFI
		}
	}
}
EOF
		  )
	bootpart=$(cat <<EOF
	partition efi {
		offset = $bootoffs
		partition-type-uuid = U
		bootable = true
		image = efi-part.vfat
	}
EOF
		  )

    fi
}

common=$(dirname $(readlink -f "$0"))
root=$BUILD_DIR/genimage.root
tmp=$BUILD_DIR/genimage.tmp

total=$((512 << M))
bootoffs=
bootimg=
bootpart=

while getopts "a:s:" opt; do
    case ${opt} in
	a)
	    arch=$OPTARG
	    ;;
	s)
	    total=$(size2int $OPTARG)
	    ;;
    esac
done
shift $((OPTIND - 1))

mkdir -p $root

probeboot
dimension
genboot

# Use awk over sed because replacement text may contain newlines,
# which sed does not approve of.
awk \
	  -vtotal=$total \
	  -vauxsize=$auxsize -vauxoffs=$auxoffs \
	  -vimgsize=$imgsize \
	  -vcfgsize=$cfgsize \
	  -vvarsize=$varsize \
	  -vbootimg="$bootimg" -vbootpart="$bootpart" \
	  '{
		sub(/@TOTALSIZE@/, total);
		sub(/@AUXSIZE@/, auxsize);
		sub(/@AUXOFFS@/, auxoffs);
		sub(/@IMGSIZE@/, imgsize);
		sub(/@CFGSIZE@/, cfgsize);
		sub(/@VARSIZE@/, varsize);

		sub(/@BOOTIMG@/, bootimg);
		sub(/@BOOTPART@/, bootpart);
	  }1' \
	      < $common/genimage.cfg.in >$root/genimage.cfg

mkdir -p $root/aux
cp -f $BINARIES_DIR/rootfs.itbh $root/aux/primary.itbh
cp -f $BINARIES_DIR/rootfs.itbh $root/aux/secondary.itbh

case "$arch" in
    aarch64)
	mkenvimage -s 0x4000 -o "$root/aux/uboot.env" \
		   "$BR2_EXTERNAL_INFIX_PATH/board/common/uboot/aux-env.txt"
	;;
    x86_64)
	mkdir -p "$root/aux/grub"
	cp -f "$BR2_EXTERNAL_INFIX_PATH/board/$arch/grub.cfg" \
	      "$BR2_EXTERNAL_INFIX_PATH/board/$arch/grubenv"  \
	      "$root/aux/grub/"
	;;
    *)
	;;
esac

rm -rf "$tmp"

genimage \
    --rootpath "$root" \
    --tmppath  "$tmp" \
    --inputpath "$BINARIES_DIR" \
    --outputpath "$BINARIES_DIR" \
    --config "$root/genimage.cfg"
