#!/bin/sh
# Build and run the confd base64 unit test, src/confd/src/base64-test.c

SCRIPT_PATH="$(dirname "$(readlink -f "$0")")"
TOPDIR="$SCRIPT_PATH/../../.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if ! ${CC:-cc} -Wall -Wextra -o "$tmp/base64-test" "$TOPDIR/src/confd/src/base64-test.c"; then
    echo "1..1"
    echo "not ok 1 - failed building base64-test"
    exit 1
fi

"$tmp/base64-test"
