#!/bin/sh
set -e

BOARD_DIR=$(dirname "$0")
GENIMAGE_CFG="${BUILD_DIR}/genimage.cfg"
GENIMAGE_TMP="${BUILD_DIR}/genimage.tmp"

# Device trees are installed for distro boot (syslinux.conf), but on RPi
# we need them for the SPL, which feeds the TPL (U-Boot) for use instead
# of the (built-in) control DT other platforms use.
find "${TARGET_DIR}/boot" -type f -name '*.dtb' -exec cp '{}' "${BINARIES_DIR}/" \;

# We've asked U-Boot previously to build overlays for us: Infix signing
# key and our ixboot scripts.  Make sure here they are installed in the
# proper directory so genimage can create the DOS partition the SPL
# reads config.txt from.
find "${BINARIES_DIR}" -type f -name '*.dtbo' -exec mv '{}' "${BINARIES_DIR}/rpi-firmware/overlays/" \;

# Create FILES array for the genimage.cfg generation
FILES=""
for f in "${BINARIES_DIR}"/*.dtb "${BINARIES_DIR}"/rpi-firmware/*; do
    case "$f" in
        *~|*.bak) continue ;;
    esac
    FILES="${FILES}\t\t\t\"${f#"${BINARIES_DIR}/"}\",\n"
done

KERNEL=$(sed -n 's/^kernel=//p' "${BINARIES_DIR}/rpi-firmware/config.txt")
FILES="${FILES}\t\t\t\"${KERNEL}\""

sed "s|#BOOT_FILES#|${FILES}|" "${BOARD_DIR}/genimage.cfg.in" > "${GENIMAGE_CFG}"

ROOTPATH_TMP=$(mktemp -d)
trap 'rm -rf \"$ROOTPATH_TMP\"' EXIT

rm -rf "${GENIMAGE_TMP}"

genimage					\
    --rootpath   "${ROOTPATH_TMP}"		\
    --tmppath    "${GENIMAGE_TMP}"		\
    --inputpath  "${BINARIES_DIR}"		\
    --outputpath "${BINARIES_DIR}"		\
    --config     "${GENIMAGE_CFG}"
