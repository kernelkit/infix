#!/bin/sh

set -e

squash="${BINARIES_DIR}"/rootfs.squashfs

itb="${BINARIES_DIR}"/rootfs.itb
itbh_size=0x1000

IFS=:
set ${KEY}
case $# in
    1)
	keyfile="$1"
	hint=$(basename ${keyfile%.*})
	;;
    2)
	keyfile="$2"
	hint="$1"
	;;
    *)
	echo "INVALID KEY" >&2
	exit 1
	;;
esac

rsanibbles=$(openssl rsa -in "${KEY}" -noout -modulus | \
		 sed -e 's/^Modulus=//' | tr -d '\n' | wc -c)
if [ "${rsanibbles}" -le 0 ]; then
    echo "ONLY RSA KEYS ARE SUPPORTED" >&2
    exit 1
fi

cat >"${WORKDIR}"/rootfs.its <<EOF
/dts-v1/;

/ {
	description = "${ARTIFACT}";
	creator = "infix";
	#address-cells = <0x1>;

	images {
		rootfs {
			description = "rootfs";
			type = "ramdisk";
			os = "linux";
			compression = "none";
			data = /incbin/("${squash}");
			signature-1 {
				algo = "sha256,rsa$((rsanibbles << 2))";
				key-name-hint = "${hint}";
			};
		};
	};

	configurations {
		default = "verity";
		verity {
			ramdisk = "rootfs";
		};
	};
};

EOF

mkimage -E -p $itbh_size -B $itbh_size \
	-f "${WORKDIR}"/rootfs.its \
	-g "${hint}" -G "${keyfile}" \
	"${itb}"

dd if="${itb}" bs=$((itbh_size)) count=1 of="${itb}h" status=none
