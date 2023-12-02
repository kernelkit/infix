#!/bin/sh

SCRIPT_PATH="$(dirname "$(readlink -f "$0")")"

if [ $# -lt 2 ]; then
    echo "Usage: $0 JSON-FILE MODULE [ ARGS ]"
    exit 1
fi

json=$1; shift
module=$1; shift

echo "1..1"

if [ ! -e "$SCRIPT_PATH/$json" ]; then
    echo "not ok 1 - $SCRIPT_PATH/$json not found"
    exit 1
fi

cat "$SCRIPT_PATH/$json" | \
    "$SCRIPT_PATH"/../../../board/netconf/rootfs/libexec/infix/cli-pretty \
    "$module" $*
if [ $? -eq 0 ]; then
    echo "ok 1 - $json printed without crashing"
    exit 0
fi

echo "not ok 1 - $json printing returned non zero"
exit 1
