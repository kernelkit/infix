#!/bin/sh
# Create new defconfigs by only applying the changes from a standard defconfig

set -e

SCRIPT_DIR="$(readlink -f $(dirname -- "$0"))"
MERGE_CONFIG="${SCRIPT_DIR}/../buildroot/support/kconfig/merge_config.sh"

usage() {
    cat <<EOF
Usage: $(basename "$0") -b BASE_CONFIG -c CHANGES... -o OUTPUT_FILE
Create a new defconfig by applying changes to a base configuration.

Required Options:
    -b, --base BASE          Path to base defconfig file
    -c, --changes CHANGES    Path to config changes to apply (can be specified multiple times)
    -o, --output OUTPUT      Path to output defconfig file

Example:
    $(basename "$0") \\
        -b configs/aarch64_defconfig \\
        -c changes/no-containers.conf \\
        -c changes/extra-features.conf \\
        -o configs/aarch64_custom_defconfig
EOF
    exit 1
}

check_file() {
    file="$1"
    type="$2"
    if [ ! -f "$file" ]; then
        echo "Error: $type file does not exist: $file" >&2
        exit 1
    elif [ ! -r "$file" ]; then
        echo "Error: $type file is not readable: $file" >&2
        exit 1
    fi
}

base=""
output=""
changes=""

while [ $# -gt 0 ]; do
    case $1 in
        -b|--base)
            base="$2"
            shift 2
            ;;
        -c|--changes)
	    changes="$changes $2"
            shift 2
            ;;
        -o|--output)
            output="$2"
            shift 2
            ;;
        *)
            usage
            exit 1
            ;;
    esac
done

if [ -z "$base" ] || [ -z "$changes" ] || [ -z "$output" ]; then
    usage
    exit 1
fi

if [ ! -x "$MERGE_CONFIG" ]; then
    echo "Error: merge_config.sh not found or not executable at: $MERGE_CONFIG"
    exit 1
fi

check_file "$base" "Base config"
for change in $changes; do
    check_file "$change" "Changes config"
done

TMPDIR=`mktemp -d`
$MERGE_CONFIG -O "$TMPDIR" "$base" "$changes"

O="$TMPDIR" make savedefconfig
mv "$TMPDIR"/defconfig "$output"
rm -r "$TMPDIR"
