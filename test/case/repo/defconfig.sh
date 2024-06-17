#!/bin/sh

SCRIPT_PATH="$(dirname "$(readlink -f "$0")")"
CONFIGS="$SCRIPT_PATH/../../../configs"

whitelist()
{
    case "$1" in
	*_boot_defconfig)
	    return 0
	    ;;
    esac

    return 1
}

# Check for disabled root login
disabled_root_login()
{
    grep -q "# BR2_TARGET_ENABLE_ROOT_LOGIN is not set" "$1"
}

# For all defconfigs
check()
{
    local total=$#
    local num=1
    local base=

    echo "1..$total"
    for defconfig in "$@"; do
	base=$(basename "$defconfig")
	if disabled_root_login "$defconfig"; then
	    echo "ok $num - $base disables root logins"
	else
	    if whitelist "$base"; then
		echo "ok $num - $base is exempted # skip"
	    else
		echo "not ok $num - $base has not disabled root login"
	    fi
	fi
	num=$((num + 1))
    done
}

check "$CONFIGS"/* || exit 1

exit 0
