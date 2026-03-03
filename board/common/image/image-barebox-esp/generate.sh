#!/bin/sh

set -e

case "$BR2_ARCH" in
    x86_64)
	BOOT_EFI=BOOTX64.EFI
	;;
    *)
	echo "Unknown EFI boot path for $BR2_ARCH" >&2
	exit 1
	;;
esac

mkdir -p "${WORKDIR}"/root
rm -rf   "${WORKDIR}"/tmp
mkdir -p "${WORKDIR}"/tmp

cat <<EOF >"${WORKDIR}"/genimage.cfg
image barebox-esp.vfat {
	size = "16M"
	vfat {
		file EFI/BOOT/$BOOT_EFI {
			image = $BINARIES_DIR/barebox.efi
		}
	}
}

# Silence genimage warnings
config {}
EOF

genimage \
    --tmppath    "${WORKDIR}"/tmp  \
    --rootpath   "${WORKDIR}"/root \
    --inputpath  "${WORKDIR}"      \
    --outputpath "${BINARIES_DIR}" \
    --config "${WORKDIR}"/genimage.cfg
