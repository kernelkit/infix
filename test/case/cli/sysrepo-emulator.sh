#!/bin/sh

SCRIPT_PATH="$(dirname "$(readlink -f "$0")")"
ROOT_PATH="$SCRIPT_PATH/../../../"

YANGER_TOOL="$ROOT_PATH/src/statd/python/yanger/yanger.py"

INTERFACES_OUTPUT_FILE="$(mktemp)"
ROUTES_OUTPUT_FILE="$(mktemp)"
cleanup() {
    rm -f "$INTERFACES_OUTPUT_FILE"
    rm -f "$ROUTES_OUTPUT_FILE"
}
trap cleanup EXIT

if [ ! -e "$YANGER_TOOL" ]; then
    echo "Error, yanger tool not found"
    exit 1
fi

if ! "$YANGER_TOOL" "ietf-interfaces" \
	-t "$SCRIPT_PATH/system-output/" >> "$INTERFACES_OUTPUT_FILE"; then
	echo "Error, running yanger for interface $iface" >&2
	exit 1
fi

$YANGER_TOOL "ietf-routing" -t "$SCRIPT_PATH/system-output/" > "$ROUTES_OUTPUT_FILE"

# Merge all module files
jq -s '.[0] * .[1]' $ROUTES_OUTPUT_FILE $INTERFACES_OUTPUT_FILE
