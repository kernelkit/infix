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

    # Place aux right after the GPT...
    auxoffs=$((32 << K))

    if [ "$bootoffs" ]; then
	if [ $((bootoffs)) -lt $((32 << K)) ]; then
	    echo "Boot partition collides with GPT"
	    exit 1
	fi

	# ...unless we have a boot partition, in which case we place
	# it after that.
	auxoffs=$((auxoffs + bootsize))
    fi

    # Finally, size var to fit whatever is left over by subtracting
    # all other images, plus another 32K at the end for the backup
    # GPT.
    varsize=$((total - auxoffs - auxsize - 2 * imgsize - cfgsize - (32 << K)))
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
