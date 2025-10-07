#!/bin/sh
set -e

usage()
{
    cat <<EOF
Create Raspberry Pi 4 SD card image from build artifacts.

Usage:
  $0 [OPTIONS]

Options:
  -h              This help text
  -b boot-dir     Path to bootloader build directory, default: O= or output/
  -r root-dir     Path to Linux rootfs.squashfs, default O= or output/

Description:
  When called without arguments (from Buildroot), uses environment variables:
    BINARIES_DIR, BUILD_DIR, BR2_EXTERNAL_INFIX_PATH, RELEASE, INFIX_ID

  When called with arguments, sets up standalone mode and combines artifacts
  from separate boot and rootfs sources.

Output:
   The resulting SD card image is saved to boot-dir/images/*-sdcard.img

Examples:
  # Using separate build directories:
  $0 -b x-boot -r output

  # Using a rootfs file directly:
  $0 -b x-boot -r ~/Downloads/rootfs.squashfs

EOF
}

getconfig()
{
    [ -f .config ] || return 1
    grep "^$1=" .config | sed -e "s/$1=\"\?\([^\"]*\)\"\?/\1/"
}

find_build_dir()
{
    # Check O= environment variable first
    if [ -n "$O" ] && [ -f "$O/.config" ]; then
        echo "$O"
        return 0
    fi

    # Check output/ directory
    if [ -f "output/.config" ]; then
        echo "output"
        return 0
    fi

    return 1
}

# Parse command line options
STANDALONE=0
while getopts "hb:r:" flag; do
    case "${flag}" in
        h) usage; exit 0;;
        b) BOOT_DIR=${OPTARG}; STANDALONE=1;;
        r) ROOT_DIR=${OPTARG}; STANDALONE=1;;
        *) usage; exit 1;;
    esac
done

# Standalone mode: set up environment from build directories
if [ "$STANDALONE" -eq 1 ] || [ $# -gt 0 ]; then
    STANDALONE=1

    # Find BR2_EXTERNAL_INFIX_PATH (current script is in src/board/raspberry-pi-4/)
    SCRIPT_DIR=$(dirname "$0")
    BR2_EXTERNAL_INFIX_PATH=$(cd "$SCRIPT_DIR/../../.." && pwd)

    # Find boot directory if not specified (try common patterns)
    if [ -z "$BOOT_DIR" ]; then
        for dir in x-boot build-boot output-boot; do
            if [ -f "$dir/.config" ]; then
                BOOT_DIR="$dir"
                break
            fi
        done
        if [ -z "$BOOT_DIR" ]; then
            BOOT_DIR=$(find_build_dir) || {
                echo "Error: Could not find boot directory. Use -b option" >&2
                exit 1
            }
        fi
    fi

    # Find rootfs if not specified
    if [ -z "$ROOT_DIR" ]; then
        ROOT_DIR=$(find_build_dir) || {
            echo "Error: Could not find rootfs directory. Set O= or use -r option" >&2
            exit 1
        }
    fi

    # Set up environment variables (use BOOT_DIR as base)
    export BINARIES_DIR="$BOOT_DIR/images"
    export BUILD_DIR="$BOOT_DIR/build"
    export BR2_EXTERNAL_INFIX_PATH
    export RELEASE=${RELEASE:-""}
    export INFIX_ID=${INFIX_ID:-"infix"}

    # Add host tools to PATH (for genimage, etc.)
    for dir in "$BOOT_DIR" "$ROOT_DIR"; do
        if [ -d "$dir/host/bin" ]; then
            export PATH="$dir/host/bin:$PATH"
            break
        fi
    done

    # Copy rootfs and partition images to boot directory
    mkdir -p "$BINARIES_DIR"
    if [ -f "$ROOT_DIR" ]; then
        # Direct path to rootfs.squashfs file
        echo "Copying rootfs from $ROOT_DIR to $BINARIES_DIR/rootfs.squashfs"
        cp "$ROOT_DIR" "$BINARIES_DIR/rootfs.squashfs"
    elif [ -f "$ROOT_DIR/images/rootfs.squashfs" ]; then
        # Build directory with images/ - copy rootfs and partition images
        echo "Copying rootfs and partitions from $ROOT_DIR/images/ to $BINARIES_DIR/"
        cp "$ROOT_DIR/images/rootfs.squashfs" "$BINARIES_DIR/"
        # Copy partition images (aux.ext4, cfg.ext4, var.ext4) if they exist
        for img in aux.ext4 cfg.ext4 var.ext4; do
            if [ -f "$ROOT_DIR/images/$img" ]; then
                cp "$ROOT_DIR/images/$img" "$BINARIES_DIR/"
            fi
        done
    elif [ -f "$ROOT_DIR/rootfs.squashfs" ]; then
        # Directory directly containing rootfs.squashfs
        echo "Copying rootfs from $ROOT_DIR/rootfs.squashfs to $BINARIES_DIR/"
        cp "$ROOT_DIR/rootfs.squashfs" "$BINARIES_DIR/"
        # Copy partition images if they exist in same directory
        for img in aux.ext4 cfg.ext4 var.ext4; do
            if [ -f "$ROOT_DIR/$img" ]; then
                cp "$ROOT_DIR/$img" "$BINARIES_DIR/"
            fi
        done
    else
        echo "Error: Could not find rootfs.squashfs in $ROOT_DIR" >&2
        exit 1
    fi
fi

BOARD_DIR=$(dirname "$0")
GENIMAGE_CFG="${BUILD_DIR}/genimage.cfg"
GENIMAGE_TMP="${BUILD_DIR}/genimage.tmp"

# We've asked U-Boot previously to build overlays for us: Infix signing
# key and our ixboot scripts.  Make sure here they are installed in the
# proper directory so genimage can create the DOS partition the SPL
# reads config.txt from.
find "${BINARIES_DIR}" -type f -name '*.dtbo' ! -path "${BINARIES_DIR}/rpi-firmware/overlays/*" -exec \
     mv '{}' "${BINARIES_DIR}/rpi-firmware/overlays/" \;

# Create FILES array for the genimage.cfg generation
FILES=""
for f in "${BINARIES_DIR}"/rpi-firmware/*; do
    case "$f" in
	*~ | *.bak)
	    continue
	    ;;
    esac
    # If already exist it has been added by us.
    echo "${FILES}" | grep -q "$(basename "$f")" && continue
    FILES="${FILES}\t\t\t\"${f#"${BINARIES_DIR}/"}\",\n"
done


FILES="${FILES}\t\t\t\"splash.bmp\",\n"

KERNEL=$(sed -n 's/^kernel=//p' "${BINARIES_DIR}/rpi-firmware/config.txt")
FILES="${FILES}\t\t\t\"${KERNEL}\""

# Create genimage.cfg from template .in
sed "s|#BOOT_FILES#|${FILES}|" "${BOARD_DIR}/genimage.cfg.in" | \
sed "s|#INFIX_ID#|${INFIX_ID}|" | \
sed "s|#VERSION#|${RELEASE}|" > "${GENIMAGE_CFG}"

# Create temporary root path
ROOTPATH_TMP=$(mktemp -d)
trap 'rm -rf \"$ROOTPATH_TMP\"' EXIT

# Clean previous genimage temp directory
rm -rf "${GENIMAGE_TMP}"

# Generate the SD card image
genimage					\
    --rootpath   "${ROOTPATH_TMP}"		\
    --tmppath    "${GENIMAGE_TMP}"		\
    --inputpath  "${BINARIES_DIR}"		\
    --outputpath "${BINARIES_DIR}"		\
    --config     "${GENIMAGE_CFG}"

# Print the resulting image path
if [ "$STANDALONE" -eq 1 ]; then
    echo ""
    echo "SD card image created successfully:"
    for img in "${BINARIES_DIR}"/*-sdcard.img; do
        if [ -f "$img" ]; then
            # Get relative path from current directory
            rel_path=$(realpath --relative-to="$PWD" "$img" 2>/dev/null || echo "$img")
            echo "  $rel_path"
        fi
    done
fi
