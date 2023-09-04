#!/bin/bash

SCRIPT_PATH="$(dirname "$(readlink -f "$0")")"

set -o pipefail

if [ $# -lt 2 ]; then
    echo "Usage: $0 JSON-FILE MODULE [ ARGS ]"
    exit 1
fi

json=$1; shift
module=$1; shift

echo "1..1"

cat "$SCRIPT_PATH/$json" | \
    "$SCRIPT_PATH"/../../../board/netconf/rootfs/lib/infix/cli-pretty \
    "$module" $*
if [ $? -eq 0 ]; then
    echo "ok 1 - $json printed without crashing"
    exit 0
fi

echo "not ok 1 - $json printing returned non zero"
exit 1
