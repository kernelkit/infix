#!/bin/sh

set -e

boot="${1}"
squash="${BINARIES_DIR}"/rootfs.squashfs
itbh="${BINARIES_DIR}"/rootfs.itbh
aux="${BINARIES_DIR}"/aux.ext4

mkdir -p "${WORKDIR}"/aux
rm -rf   "${WORKDIR}"/tmp
mkdir -p "${WORKDIR}"/tmp

cp -f "${itbh}" "${WORKDIR}"/aux/primary.itbh
cp -f "${itbh}" "${WORKDIR}"/aux/secondary.itbh

tstamp=$(date -u +%FT%TZ)
rootsha=$(sha256sum "${squash}" | cut -d" " -f1)
rootsize=$(stat -c %s "${squash}")
cat <<EOF >"${WORKDIR}"/aux/rauc.status
[slot.rootfs.0]
bundle.compatible=${COMPATIBLE}
bundle.version=${VERSION}
status=ok
sha256=${rootsha}
size=${rootsize}
installed.timestamp=$tstamp
installed.count=1
activated.timestamp=$tstamp
activated.count=1

[slot.rootfs.1]
bundle.compatible=${COMPATIBLE}
bundle.version=${VERSION}
status=ok
sha256=${rootsha}
size=${rootsize}
installed.timestamp=$tstamp
installed.count=1
activated.timestamp=$tstamp
activated.count=1
EOF

case "${boot}" in
    uboot)
	cat <<EOF | mkenvimage -s 0x4000 -o "${WORKDIR}"/aux/uboot.env -
BOOT_ORDER=primary secondary net
BOOT_primary_LEFT=1
BOOT_secondary_LEFT=1
BOOT_net_LEFT=1
EOF
	;;
    grub)
	mkdir -p "${WORKDIR}"/aux/grub
	cp -f "${PKGDIR}"/grub.cfg "${PKGDIR}"/grubenv "${WORKDIR}"/aux/grub
	;;
    *)
	echo "UNSUPPORTED BOOTLOADER ${boot}" >&2
	exit 1
esac

cat <<EOF >"${WORKDIR}"/genimage.cfg
image $(basename ${aux}) {
	mountpoint = "/"
	size = 8M

	ext4 {
		label = "aux"
		use-mke2fs = true
		features = "^metadata_csum,^metadata_csum_seed,uninit_bg"
                extraargs = "-m 0 -i 4096"
	}
}

# Silence genimage warnings
config {}
EOF

genimage \
    --loglevel 1 \
    --tmppath    "${WORKDIR}"/tmp \
    --rootpath   "${WORKDIR}"/aux \
    --inputpath  "${WORKDIR}" \
    --outputpath "$(dirname ${aux})" \
    --config     "${WORKDIR}"/genimage.cfg
