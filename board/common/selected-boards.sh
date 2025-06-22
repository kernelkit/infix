#!/bin/bash
#
# Check if an infix board is selected

if [ $# -ne 2 ]; then
	echo "usage: $0 <INFIX-DIR> <OUTPUT>"
	exit 1
fi
ROOT=$1
O=$2
BOARD_SRC_DIR="${ROOT}/src/board"
BOARD_PACKAGE_DIR="${ROOT}/package/board"
CONFIG_FILE="${O}/.config"

is_board_enabled() {
    local symbol=$1

    if [ -f "$CONFIG_FILE" ]; then
        if grep -q "^${symbol}=y" "$CONFIG_FILE" 2>/dev/null; then
            return 0  # enabled
        else
            return 1  # disabled or not set
        fi
    else
        echo "Warning: No .config file found. Run 'make menuconfig' first."
        return 1
    fi
}

get_actual_config_symbol() {
    local board_name=$1
    local config_file="${BOARD_PACKAGE_DIR}/$board_name/Config.in"

    if [ -f "$config_file" ]; then
        # Extract the first config symbol from the Config.in file
        local symbol=$(grep -m1 "^config " "$config_file" 2>/dev/null | awk '{print $2}')
        if [ -n "$symbol" ]; then
            echo "$symbol"
            return 0
        fi
    fi

    # Fallback to predicted symbol if no Config.in found
    echo "BR2_PACKAGE_BOARD_$(echo "$board_name" | tr '[:lower:]' '[:upper:]' | tr '-' '_')"
    return 1
}

boards=""

for board_path in "$BOARD_SRC_DIR"/*; do
    if [ -d "$board_path" ]; then
        board_name=$(basename "$board_path")
        config_symbol=$(get_actual_config_symbol "$board_name")

        if is_board_enabled "$config_symbol"; then
	    boards="$boards $board_name"
        fi
    fi
done

echo "$boards"
