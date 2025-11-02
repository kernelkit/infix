#!/bin/sh
# Unified SD card image creation script for all boards
# Consolidates logic from board-specific mkimage.sh scripts
set -e

STANDALONE=""
if [ -z "$BR2_EXTERNAL_INFIX_PATH" ]; then
    SCRIPT_DIR=$(dirname "$0")
    BR2_EXTERNAL_INFIX_PATH=$(cd "$SCRIPT_DIR/.." && pwd)
fi

usage()
{
    cat <<EOF
Create SD card image from build artifacts for any supported board.

Usage:
  $0 [OPTIONS] <board-name>

Options:
  -b boot-dir     Path to bootloader build directory (default: O= or output/)
  -d              Download bootloader files from latest-boot release
  -f              Force re-download of bootloader even if cached
  -h              This help text
  -l              List available boards
  -o              Override auto-detection of genimage.sh, use host installed version
  -r root-dir     Path to rootfs build directory or rootfs.squashfs file (default: O= or output/)

Arguments:
  board-name      Board identifier (must come after options)

Description:
  When called from Buildroot (no options), uses environment variables:
    BINARIES_DIR, BUILD_DIR, BR2_EXTERNAL_INFIX_PATH, RELEASE, INFIX_ID

  When called with -b/-r options, enters standalone mode and combines artifacts
  from separate boot and rootfs sources. Useful for CI or manual image creation.

Output:
  SD card image saved to \$BINARIES_DIR/*-sdcard.img

Examples:
  # From Buildroot build:
  $0 raspberrypi-rpi64

  # Standalone with separate boot/rootfs builds:
  $0 -b x-boot -r output raspberrypi-rpi64

  # With downloaded rootfs and bootloader:
  $0 -d -r ~/Downloads/rootfs.squashfs friendlyarm-nanopi-r2s

  # Download bootloader and compose with Linux image in output directory:
  $0 -od bananapi-bpi-r3

EOF
}

log()
{
    printf '\033[7m>>> %s\033[0m\n' "$*"
}

err()
{
    echo "Error: $*" >&2
}

die()
{
    err "$*"
    exit 1
}

# List all supported boards by scanning for genimage.cfg.in files
list_boards()
{
    script_dir=$(dirname "$0")
    board_base=$(cd "$script_dir/../board" 2>/dev/null && pwd)

    if [ -z "$board_base" ] || [ ! -d "$board_base" ]; then
        echo "Error: Could not find board directory" >&2
        return 1
    fi

    echo "Available boards:"
    find "$board_base" -name "genimage.cfg.in" -type f 2>/dev/null | \
        grep -v '/common/' | \
        sed 's|.*/board/\([^/]*\)/\([^/]*\)/.*|  \2 (\1)|' | \
        sort -u
}

# Run genimage directly (fallback when Buildroot wrapper not available)
run_genimage()
{
    genimage_cfg="$1"
    genimage_tmp="${BUILD_DIR}/genimage.tmp"
    rootpath_tmp=$(mktemp -d)
    trap 'rm -rf "$rootpath_tmp"' EXIT

    rm -rf "$genimage_tmp"

    genimage \
        --rootpath   "$rootpath_tmp" \
        --tmppath    "$genimage_tmp" \
        --inputpath  "${BINARIES_DIR}" \
        --outputpath "${BINARIES_DIR}" \
        --config     "$genimage_cfg"

    if command -v bmaptool >/dev/null 2>&1; then
        for img in "${BINARIES_DIR}"/*-sdcard.img; do
            [ -f "$img" ] || continue
            log "Generating block map for $(basename "$img")..."
            bmaptool create -o "${img}.bmap" "$img"
        done
    fi
}

# Validate board argument and find board directory
# Sets BOARD and BOARD_DIR globals or exits with error
validate_board()
{
    BOARD="$1"
    if [ -z "$BOARD" ]; then
        err "Board name required. Use -h for help."
	return 1
    fi

    board_underscore=$(echo "$BOARD" | tr '-' '_')
    for arch in aarch64 x86_64 riscv64; do
        for variant in "$BOARD" "$board_underscore"; do
            candidate="$BR2_EXTERNAL_INFIX_PATH/board/$arch/$variant"
            if [ -d "$candidate" ]; then
                BOARD_DIR="$candidate"
                return 0
            fi
        done
    done

    err "Board directory not found for: $BOARD"
    return 1
}

# Find build directory by checking O= or output/
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

# Map board name to bootloader identifier
# Returns bootloader name used in artifact naming
get_bootloader_name()
{
    board="$1"
    case "$board" in
        raspberrypi-rpi64)
            echo "rpi64_boot"
            ;;
        bananapi-bpi-r3)
            echo "bpi_r3_boot"
            ;;
        friendlyarm-nanopi-r2s)
            echo "nanopi_r2s_boot"
            ;;
        *)
            err "Unknown bootloader for board: $board"
            return 1
            ;;
    esac
}

# Download and extract bootloader from latest-boot release
# Downloads to dl/bootloader/ cache and extracts to temporary build directory
# Returns the temporary directory path in SDCARD_TEMP_DIR variable
download_bootloader()
{
    board="$1"
    build_dir="$2"

    bootloader=$(get_bootloader_name "$board") || return 1

    if ! command -v gh >/dev/null 2>&1; then
        die "gh CLI not found. Install it or build bootloader locally."
    fi

    # Set up download cache directory
    dl_dir="${BR2_EXTERNAL_INFIX_PATH}/dl/bootloader"
    mkdir -p "$dl_dir"

    # Convert underscores to dashes for filename pattern matching
    bootloader_pattern=$(echo "$bootloader" | tr '_' '-')

    # Find or download bootloader tarball
    tarball=$(ls "$dl_dir"/${bootloader_pattern}*.tar.gz 2>/dev/null | head -n1)

    if [ -z "$tarball" ] || [ -n "$FORCE_DOWNLOAD" ]; then
        if [ -n "$FORCE_DOWNLOAD" ] && [ -n "$tarball" ]; then
            log "Force re-downloading bootloader..."
            rm -f "$tarball" "$tarball.sha256"
        else
            log "Downloading bootloader $bootloader from latest-boot release..."
        fi

        if ! gh release download latest-boot \
             --repo kernelkit/infix \
             --pattern "*${bootloader_pattern}*.tar.gz" \
             --dir "$dl_dir"; then
            die "Failed downloading bootloader from latest-boot release. Check gh authentication and network connectivity."
        fi

        tarball=$(ls "$dl_dir"/${bootloader_pattern}*.tar.gz 2>/dev/null | head -n1)
        [ -n "$tarball" ] || die "Downloaded tarball not found in $dl_dir"
    else
        log "Using cached bootloader: $(basename "$tarball")"
    fi

    # Create temporary directory for SD card composition
    SDCARD_TEMP_DIR="${build_dir}/sdcard-${board}-$$"
    mkdir -p "$SDCARD_TEMP_DIR"

    log "Extracting bootloader to $SDCARD_TEMP_DIR..."
    tar -xzf "$tarball" --strip-components=1 -C "$SDCARD_TEMP_DIR/"

    log "Bootloader ready in temporary directory"
}

# Discover boot files for Raspberry Pi boot partition
# Scans rpi-firmware directory and builds file list for genimage
discover_rpi_boot_files()
{
    binaries_dir="$1"
    files=""

    # Move any .dtbo overlays to proper location
    find "${binaries_dir}" -type f -name '*.dtbo' ! -path "${binaries_dir}/rpi-firmware/overlays/*" -exec \
         mv '{}' "${binaries_dir}/rpi-firmware/overlays/" \; 2>/dev/null || true

    # Scan rpi-firmware directory for boot files
    for f in "${binaries_dir}"/rpi-firmware/*; do
        [ -e "$f" ] || continue
        case "$f" in
            *~ | *.bak)
                continue
                ;;
        esac
        # Skip if already in list
        echo "${files}" | grep -q "$(basename "$f")" && continue
        files="${files}\t\t\t\"${f#"${binaries_dir}/"}\",\n"
    done

    # Add splash screen
    files="${files}\t\t\t\"splash.bmp\",\n"

    # Add kernel (extract name from config.txt)
    if [ -f "${binaries_dir}/rpi-firmware/config.txt" ]; then
        kernel=$(sed -n 's/^kernel=//p' "${binaries_dir}/rpi-firmware/config.txt")
        files="${files}\t\t\t\"${kernel}\""
    fi

    echo "$files"
}

while getopts "hldfob:r:" opt; do
    case $opt in
        b)
	    BOOT_DIR="$OPTARG"
	    STANDALONE=1
	    ;;
	d)
	    DOWNLOAD_BOOT=1
	    STANDALONE=1
	    ;;
	f)
	    FORCE_DOWNLOAD=1
	    ;;
        h)
	    usage
	    exit 0
	    ;;
        l)
	    list_boards
	    exit 0
	    ;;
	o)
	    OVERRIDE=1
	    ;;
        r)
	    ROOT_DIR="$OPTARG"
	    STANDALONE=1
	    ;;
        *)
	    usage
	    exit 1
	    ;;
    esac
done
shift $((OPTIND - 1))

if ! validate_board "$1"; then
    usage
    exit 1
fi

# Standalone mode: set up environment from build directories
if [ -n "$STANDALONE" ]; then
    # In download mode without explicit dirs, default to same location for both
    if [ -n "$DOWNLOAD_BOOT" ]; then
        default_dir=$(find_build_dir) || die "Could not find build directory. Set O= or use -b/-r option"
        : "${BOOT_DIR:=$default_dir}"
        : "${ROOT_DIR:=$default_dir}"
    else
        if [ -z "$BOOT_DIR" ]; then
            BOOT_DIR=$(find_build_dir) || die "Could not find boot directory. Use -b option"
        fi

        if [ -z "$ROOT_DIR" ]; then
            ROOT_DIR=$(find_build_dir) || die "Could not find rootfs directory. Set O= or use -r option"
        fi
    fi

    # Set up environment variables, some required by genimage.sh
    export BINARIES_DIR="$BOOT_DIR/images"
    export BUILD_DIR="$BOOT_DIR/build"
    export BR2_EXTERNAL_INFIX_PATH
    export RELEASE="${RELEASE:-""}"
    export INFIX_ID="${INFIX_ID:-"infix"}"

    # Add host tools to PATH (for genimage, bmaptool, etc.)
    for dir in "$BOOT_DIR" "$ROOT_DIR"; do
        if [ -d "$dir/host/bin" ]; then
            export PATH="$dir/host/bin:$PATH"
            break
        fi
    done

    # Copy rootfs and partition images to BINARIES_DIR (skip if same directory)
    mkdir -p "$BINARIES_DIR"

    # Normalize paths for comparison
    boot_images=$(cd "$BOOT_DIR" && pwd)/images
    root_images=""

    if [ -f "$ROOT_DIR" ]; then
        # Direct path to rootfs.squashfs file
        log "Copying rootfs from $ROOT_DIR to $BINARIES_DIR/rootfs.squashfs"
        cp "$ROOT_DIR" "$BINARIES_DIR/rootfs.squashfs"
    elif [ -f "$ROOT_DIR/images/rootfs.squashfs" ]; then
        root_images=$(cd "$ROOT_DIR" && pwd)/images
        # Only copy if different directories
        if [ "$boot_images" != "$root_images" ]; then
            # Build directory with images/ - copy rootfs and partition images
            log "Copying artifacts from $ROOT_DIR/images/ to $BINARIES_DIR/"
            cp "$ROOT_DIR/images/rootfs.squashfs" "$BINARIES_DIR/"
            # Copy partition images if they exist
            for img in aux.ext4 cfg.ext4 var.ext4; do
                if [ -f "$ROOT_DIR/images/$img" ]; then
                    cp "$ROOT_DIR/images/$img" "$BINARIES_DIR/"
                fi
            done
        else
            log "Rootfs already in place at $BINARIES_DIR/"
        fi
    elif [ -f "$ROOT_DIR/rootfs.squashfs" ]; then
        # Directory directly containing rootfs.squashfs
        log "Copying rootfs from $ROOT_DIR/rootfs.squashfs"
        cp "$ROOT_DIR/rootfs.squashfs" "$BINARIES_DIR/"
        # Copy partition images if they exist
        for img in aux.ext4 cfg.ext4 var.ext4; do
            if [ -f "$ROOT_DIR/$img" ]; then
                cp "$ROOT_DIR/$img" "$BINARIES_DIR/"
            fi
        done
    else
        die "Could not find rootfs.squashfs in $ROOT_DIR"
    fi
else
    # Export for Buildroot genimage.sh wrapper
    export BINARIES_DIR
    export BUILD_DIR
fi

# Validate required environment variables
: "${BINARIES_DIR:?'not set'}"
: "${BUILD_DIR:?'not set'}"
: "${BR2_EXTERNAL_INFIX_PATH:?'not set'}"

# Set defaults for optional variables
: "${RELEASE:=""}"
: "${INFIX_ID:="infix"}"

# Download bootloader if requested
if [ -n "$DOWNLOAD_BOOT" ]; then
    # Save original output location
    ORIGINAL_BINARIES_DIR="$BINARIES_DIR"

    download_bootloader "$BOARD" "$BUILD_DIR"

    # Now use the temporary directory for composition
    BINARIES_DIR="$SDCARD_TEMP_DIR"

    log "Linking rootfs files to $BINARIES_DIR..."
    # Link rootfs and partition images to temp directory
    if [ -f "$ROOT_DIR" ]; then
        # Direct path to rootfs.squashfs file
        ln -sf "$(realpath "$ROOT_DIR")" "$BINARIES_DIR/rootfs.squashfs"
    elif [ -f "$ROOT_DIR/images/rootfs.squashfs" ]; then
        ln -sf "$(realpath "$ROOT_DIR/images/rootfs.squashfs")" "$BINARIES_DIR/rootfs.squashfs"
        # Link partition images if they exist
        for img in aux.ext4 cfg.ext4 var.ext4; do
            if [ -f "$ROOT_DIR/images/$img" ]; then
                ln -sf "$(realpath "$ROOT_DIR/images/$img")" "$BINARIES_DIR/$img"
            fi
        done
    elif [ -f "$ROOT_DIR/rootfs.squashfs" ]; then
        ln -sf "$(realpath "$ROOT_DIR/rootfs.squashfs")" "$BINARIES_DIR/rootfs.squashfs"
        # Link partition images if they exist
        for img in aux.ext4 cfg.ext4 var.ext4; do
            if [ -f "$ROOT_DIR/$img" ]; then
                ln -sf "$(realpath "$ROOT_DIR/$img")" "$BINARIES_DIR/$img"
            fi
        done
    else
        die "Could not find rootfs.squashfs in $ROOT_DIR"
    fi
fi

# Template expansion
log "Generating genimage configuration for $BOARD..."

GENIMAGE_CFG="${BUILD_DIR}/genimage.cfg"
GENIMAGE_TEMPLATE="$BOARD_DIR/genimage.cfg.in"
[ -f "$GENIMAGE_TEMPLATE" ] || die "genimage.cfg.in not found in $BOARD_DIR"

# Check if board needs special boot file discovery (Raspberry Pi)
if [ "$BOARD" = "raspberrypi-rpi64" ] && grep -q '#BOOT_FILES#' "$GENIMAGE_TEMPLATE"; then
    log "Discovering Raspberry Pi boot files..."
    BOOT_FILES=$(discover_rpi_boot_files "$BINARIES_DIR")
    # Create temp file with interpreted escape sequences
    bootfiles_tmp="${BUILD_DIR}/bootfiles.tmp"
    printf '%b' "$BOOT_FILES" > "$bootfiles_tmp"
    # Use sed to insert content and delete placeholder
    sed -e "/#BOOT_FILES#/r $bootfiles_tmp" -e "/#BOOT_FILES#/d" "$GENIMAGE_TEMPLATE" > "${GENIMAGE_CFG}.tmp"
    rm -f "$bootfiles_tmp"
    GENIMAGE_TEMPLATE="${GENIMAGE_CFG}.tmp"
fi

# Epxand template variables
sed "s|#VERSION#|${RELEASE}|" "$GENIMAGE_TEMPLATE" | \
sed "s|#INFIX_ID#|${INFIX_ID}|" | \
sed "s|#TARGET#|sd|" > "$GENIMAGE_CFG"

# Clean up temp file if created
rm -f "${GENIMAGE_CFG}.tmp"

# Find and set up for calling genimage/genimage.sh
if [ -z "$BR2_CONFIG" ]; then
    BR2_CONFIG="/dev/null"
    if [ -f "${BUILD_DIR}/../.config" ]; then
	BR2_CONFIG="$(realpath "${BUILD_DIR}/../.config")"
    fi
    export BR2_CONFIG

    if [ -f "$BR2_EXTERNAL_INFIX_PATH/../buildroot/support/scripts/genimage.sh" ]; then
	GENIMAGE_WRAPPER="$(realpath "$BR2_EXTERNAL_INFIX_PATH/../buildroot/support/scripts/genimage.sh")"
    fi
else
    GENIMAGE_WRAPPER="support/scripts/genimage.sh"
fi

if [ -z "$OVERRIDE" ] && command -v "$GENIMAGE_WRAPPER" >/dev/null 2>&1; then
    log "Creating SD card image using Buildroot $(basename "$GENIMAGE_WRAPPER") ..."
    "$GENIMAGE_WRAPPER" -c "$GENIMAGE_CFG"
else
    log "Creating SD card image ..."
    run_genimage "$GENIMAGE_CFG"
fi

# Post-processing: move images and cleanup if using download mode
if [ -n "$DOWNLOAD_BOOT" ]; then
    log "Moving SD card images to $ORIGINAL_BINARIES_DIR..."
    mkdir -p "$ORIGINAL_BINARIES_DIR"

    for img in "${BINARIES_DIR}"/*-sdcard.img*; do
        if [ -f "$img" ]; then
            mv "$img" "$ORIGINAL_BINARIES_DIR/"
            log "  $(basename "$img")"
        fi
    done

    log "Cleaning up temporary directory..."
    rm -rf "$SDCARD_TEMP_DIR"

    # Update BINARIES_DIR for final output message
    BINARIES_DIR="$ORIGINAL_BINARIES_DIR"
fi

log "SD card image created successfully:"
for img in "${BINARIES_DIR}"/*-sdcard.img*; do
    if [ -f "$img" ]; then
        if [ -n "$STANDALONE" ]; then
            # Show relative path in standalone mode
            rel_path=$(realpath --relative-to="$PWD" "$img" 2>/dev/null || echo "$img")
            echo "  $rel_path"
        else
            echo "  $(basename "$img")"
        fi
    fi
done
