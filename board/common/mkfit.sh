#!/bin/sh

die()
{
    echo "$@" >&2
    exit 1
}

load_cfg()
{
    local tmp=$(mktemp -p /tmp)

    grep ^FIT_ $BR2_CONFIG >$tmp
    .  $tmp
    rm $tmp
}

load_cfg
[ "$FIT_IMAGE" = "y" ] || {
    echo "FIT_IMAGE not set, skipping" >&2
    exit 0
}

work=$(pwd)/build/fit-image-local
dtbs=$(find $BINARIES_DIR -name '*.dtb')
kernel=$(find $BINARIES_DIR -name '*Image' | head -n1)
squash=$BINARIES_DIR/rootfs.squashfs

mkdir -p $work

# mkimage will only align images to 4 bytes, but U-Boot will leave
# both DTB and ramdisk in place when starting the kernel. So we pad
# all components up to a 4k boundary.
truncate -s %4k $kernel $dtbs

: >$work/dtbs.itsi
: >$work/cfgs.itsi
for dtb in $dtbs; do
    name=$(basename $dtb .dtb)

    cat <<EOF >>$work/dtbs.itsi
		$name-dtb {
			description = "$name";
			type = "flat_dt";
			arch = "$FIT_ARCH";
			compression = "none";
			data = /incbin/("$dtb");
		};
EOF
    cat <<EOF >>$work/cfgs.itsi
		$name {
			description = "$name";
			kernel = "kernel";
			ramdisk = "ramdisk";
			fdt = "$name-dtb";
		};
EOF
done

: >$work/kernel-load.itsi
if [ "$FIT_KERNEL_LOAD_ADDR" ]; then
    cat <<EOF >$work/kernel-load.itsi
			load = <$FIT_KERNEL_LOAD_ADDR>;
			entry = <$FIT_KERNEL_LOAD_ADDR>;
EOF
fi

cat <<EOF >$work/infix.its
/dts-v1/;

/ {
	timestamp = <$(date +%s)>;
	description = "Inf/IX ($FIT_ARCH)";
	creator = "infix";
	#address-cells = <0x1>;

	images {

		kernel {
			description = "Linux";
			type = "kernel";
			arch = "$FIT_ARCH";
			os = "linux";
$(cat $work/kernel-load.itsi)
			compression = "none";
			data = /incbin/("$kernel");
		};

		ramdisk {
			description = "Inf/IX";
			type = "ramdisk";
			os = "linux";
			arch = "$FIT_ARCH";
			compression = "none";
			data = /incbin/("$squash");
		};

$(cat $work/dtbs.itsi)

	};

	configurations {
$(cat $work/cfgs.itsi)
	};
};
EOF

mkimage \
    -E -p 0x1000 \
    -f $work/infix.its $BINARIES_DIR/infix.itb \
    || die "Unable to create FIT image"
