#!/bin/sh
set -e

BOARD_DIR=$(dirname "$0")
GENIMAGE_CFG="${BUILD_DIR}/genimage.cfg"
GENIMAGE_TMP="${BUILD_DIR}/genimage.tmp"

# We've asked U-Boot previously to build overlays for us: Infix signing
# key and our ixboot scripts.  Make sure here they are installed in the
# proper directory so genimage can create the DOS partition the SPL
# reads config.txt from.
find "${BINARIES_DIR}" -type f -name '*.dtbo' -exec mv '{}' "${BINARIES_DIR}/rpi-firmware/overlays/" \;

# Create FILES array for the genimage.cfg generation
FILES=""
for f in "${BINARIES_DIR}"/rpi-firmware/*; do
    case "$f" in
        *~|*.bak) continue ;;
    esac
    echo "${FILES}" | grep -q `basename $f` && continue # If already exist it has been added by us.
    FILES="${FILES}\t\t\t\"${f#"${BINARIES_DIR}/"}\",\n"
done
FILES="${FILES}\t\t\t\"splash.bmp\",\n"
echo $FILES
KERNEL=$(sed -n 's/^kernel=//p' "${BINARIES_DIR}/rpi-firmware/config.txt")
FILES="${FILES}\t\t\t\"${KERNEL}\""


sed "s|#BOOT_FILES#|${FILES}|" "${BOARD_DIR}/genimage.cfg.in" | \
sed "s|#INFIX_ID#|${INFIX_ID}|" | \
sed "s|#VERSION#|${RELEASE}|" > "${GENIMAGE_CFG}"


ROOTPATH_TMP=$(mktemp -d)
trap 'rm -rf \"$ROOTPATH_TMP\"' EXIT

rm -rf "${GENIMAGE_TMP}"

genimage					\
    --rootpath   "${ROOTPATH_TMP}"		\
    --tmppath    "${GENIMAGE_TMP}"		\
    --inputpath  "${BINARIES_DIR}"		\
    --outputpath "${BINARIES_DIR}"		\
    --config     "${GENIMAGE_CFG}"
