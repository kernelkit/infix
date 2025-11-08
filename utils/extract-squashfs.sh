#!/bin/sh
#
# Extract rootfs.squashfs from a release tarball's rootfs.itb file
#
# Usage: extract-squashfs.sh <tarball-or-itb-file> [output-file]
#

set -e

usage() {
    cat <<EOF
Usage: $(basename "$0") <tarball-or-itb-file> [output-file]

Extract rootfs.squashfs from a release tarball or .itb file.

Arguments:
  <tarball-or-itb-file>  Release tarball (*.tar.gz) or rootfs.itb file
  [output-file]          Output squashfs file (default: rootfs.squashfs)

Examples:
  # From release tarball
  $(basename "$0") infix-x86_64-25.09.0.tar.gz

  # From .itb file directly
  $(basename "$0") output/images/rootfs.itb

  # Custom output location
  $(basename "$0") infix-x86_64-25.09.0.tar.gz /tmp/my-rootfs.squashfs

The script skips the 4096-byte FIT header to extract the SquashFS image.
EOF
}

die() {
    echo "ERROR: $*" >&2
    exit 1
}

# Parse arguments
case "$1" in
    -h|--help)
        usage
        exit 0
        ;;
    "")
        usage
        exit 1
        ;;
esac

input="$1"
output="${2:-rootfs.squashfs}"

[ -f "$input" ] || die "File not found: $input"

# Detect if input is a tarball or .itb file
case "$input" in
    *.tar.gz)
        echo "Extracting rootfs.itb from tarball..."
        itb_file=$(tar -tzf "$input" | grep -m1 'rootfs\.itb$') \
            || die "No rootfs.itb found in tarball"

        echo "Found: $itb_file"
        echo "Extracting SquashFS (skipping 4096-byte FIT header)..."

        tar -xzOf "$input" "$itb_file" | dd bs=4096 skip=1 of="$output" 2>/dev/null \
            || die "Failed to extract SquashFS"
        ;;
    *.itb)
        echo "Extracting SquashFS from .itb file (skipping 4096-byte FIT header)..."
        dd if="$input" of="$output" bs=4096 skip=1 2>/dev/null \
            || die "Failed to extract SquashFS"
        ;;
    *)
        die "Unsupported file type. Expected *.tar.gz or *.itb"
        ;;
esac

# Verify output
if [ -f "$output" ]; then
    size=$(stat --format=%s "$output" 2>/dev/null || stat -f%z "$output")
    file_type=$(file "$output")

    # Try to format size nicely, fall back to bytes if numfmt not available
    if command -v numfmt >/dev/null 2>&1; then
        size_str=$(numfmt --to=iec-i --suffix=B "$size")
    else
        size_str="$size bytes"
    fi

    echo "Success! Created: $output ($size_str)"
    echo "Type: $file_type"

    # Quick sanity check for squashfs magic
    if echo "$file_type" | grep -qi squashfs; then
        echo "✓ Verified: Valid SquashFS filesystem"
    else
        echo "⚠ Warning: Output may not be a valid SquashFS image"
        exit 1
    fi
else
    die "Failed to create output file"
fi
