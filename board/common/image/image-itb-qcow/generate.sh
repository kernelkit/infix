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
	bootsize=$(( 8 << M))
	auxsize=$((  8 << M))
	imgsize=$((256 << M))
	cfgsize=$(( 64 << M))
	# var is at least ~0.5G
    elif [ $total -ge $((512 << M)) ]; then
	bootsize=$(( 8 << M))
	auxsize=$((  8 << M))
	imgsize=$((192 << M))
	cfgsize=$(( 16 << M))
	# var is at least ~100M
    else
	echo "Can't create disk images smaller than 512M"
	exit 1
    fi

    # Size var to fit whatever is left over by subtracting all other
    # images...
    varsize=$(($total - $auxsize - 2 * $imgsize - $cfgsize))

    # ...any space needed by bootloader + GPT...
    if [ "$bootoffs" ]; then
	# Align the end of the boot partition to an even MiB. E.g. if
	# boot was dimensioned to 4M, and bootoffs is 32K, then the
	# final bootsize becomes 4M - 32K, meaning aux will start on
	# exactly 4M.
	varsize=$(($varsize - $bootsize))
	auxoffs=$bootsize
	bootsize=$(($bootsize - $bootoffs))
    else
	# No bootloader, place aux after GPT, resize it to end on an
	# even MiB (as is done for boot above).
	varsize=$(($varsize - (32 << K)))
	auxoffs=$((32 << K))
	auxsize=$(($auxsize - $auxoffs))
    fi

    # ...plus another 32K at the end to make room for the backup GPT
    varsize=$(($varsize - (32 << K)))
}

genboot()
{
    if [ -d "$bootdata" ]; then
	bootimg=$(cat <<EOF
image efi-part.vfat {
	temporary = true
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

    elif [ -f "$bootdata" ]; then
	bootpart=$(cat <<EOF
	partition boot {
		offset = $bootoffs
		partition-type-uuid = U
		bootable = true
		image = $bootdata
		size = $bootsize
	}
EOF
		   )
    fi
}

mkdir -p "${WORKDIR}"/root
rm -rf   "${WORKDIR}"/tmp
mkdir -p "${WORKDIR}"/tmp

qcowimg="${ARTIFACT}.qcow2"
total=$(size2int $SIZE)
bootoffs=
bootdata=$BOOT_DATA
bootimg=
bootpart=

if [ -n "${BOOT_OFFSET}" ]; then
    bootoffs=$(($BOOT_OFFSET))
fi

dimension
genboot

. $BR2_EXTERNAL_INFIX_PATH/board/common/rootfs/etc/partition-uuid
[ -n "${AUX_UUID}" ]
[ -n "${PRIMARY_UUID}" ]
[ -n "${SECONDARY_UUID}" ]

# Use awk over sed because replacement text may contain newlines,
# which sed does not approve of.
awk \
   	  -vauxuuid=$AUX_UUID \
	  -vprimaryuuid=$PRIMARY_UUID \
          -vsecondaryuuid=$SECONDARY_UUID \
	  -vtotal=$total \
	  -vauxsize=$auxsize -vauxoffs=$auxoffs \
	  -vimgsize=$imgsize \
	  -vcfgsize=$cfgsize \
	  -vvarsize=$varsize \
	  -vqcowimg=$qcowimg \
	  -vbootimg="$bootimg" -vbootpart="$bootpart" \
	  '{
		sub(/@TOTALSIZE@/, total);
		sub(/@AUXSIZE@/, auxsize);
		sub(/@AUXOFFS@/, auxoffs);
		sub(/@IMGSIZE@/, imgsize);
		sub(/@CFGSIZE@/, cfgsize);
		sub(/@VARSIZE@/, varsize);
		sub(/@QCOWIMG@/, qcowimg);
		sub(/@BOOTIMG@/, bootimg);
		sub(/@BOOTPART@/, bootpart);
		sub(/@AUXUUID@/, auxuuid);
		sub(/@PRIMARYUUID@/, primaryuuid);
		sub(/@SECONDARYUUID@/, secondaryuuid);

	  }1' \
	      < $PKGDIR/genimage.cfg.in >$WORKDIR/genimage.cfg

genimage \
    --tmppath    "${WORKDIR}"/tmp  \
    --rootpath   "${WORKDIR}"/root \
    --inputpath  "$BINARIES_DIR"   \
    --outputpath "$BINARIES_DIR"   \
    --config "${WORKDIR}"/genimage.cfg
