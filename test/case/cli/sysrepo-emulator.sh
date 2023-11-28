#!/bin/sh

SCRIPT_PATH="$(dirname "$(readlink -f "$0")")"
ROOT_PATH="$SCRIPT_PATH/../../../"

YANGER_TOOL="$ROOT_PATH/board/netconf/rootfs/libexec/infix/yanger"

INTERFACES="br0 e0 e1 e2 e3 e4"

YANGER_OUTPUT_FILE="$(mktemp)"

cleanup() {
    rm -f "$YANGER_OUTPUT_FILE"
}
trap cleanup EXIT

if [ ! -e "$YANGER_TOOL" ]; then
    echo "Error, yanger tool not found"
    exit 1
fi

for iface in $INTERFACES; do
    if ! "$YANGER_TOOL" "ietf-interfaces" \
          -t "$SCRIPT_PATH/system-output/" \
          -p "$iface" >> "$YANGER_OUTPUT_FILE"; then
        echo "Error, running yanger for interface $iface" >&2
        exit 1
    fi
done

if ! jq -s 'reduce .[] as $item
       ({}; .["ietf-interfaces:interfaces"].interface +=
        $item["ietf-interfaces:interfaces"].interface)' \
      "$YANGER_OUTPUT_FILE"; then
    echo "Error, merging yanger output data" >&2
    exit 1
fi
