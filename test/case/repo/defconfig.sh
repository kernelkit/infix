#!/bin/sh

SCRIPT_PATH="$(dirname "$(readlink -f "$0")")"
CONFIGS="$SCRIPT_PATH/../../../configs"

whitelist()
{
    nm=$(basename "$1")
    case $nm in
	aarch64_classic_defconfig | cn9130_crb_boot_defconfig | fireant_boot_defconfig | x86_64_classic_defconfig)
	    return 0
	    ;;
	*)
	    return 1
	    ;;
    esac
}

# Check for disabled root login
disabled_root_login()
{
    txt="# BR2_TARGET_ENABLE_ROOT_LOGIN is not set"
    fn=$(realpath "$1")

    if ! grep -q "$txt" "$fn"; then
	if ! whitelist "$fn"; then
	    echo "Missing '$txt' in $fn!"
	    return 1
	fi
    fi
}

# For all defconfigs
check()
{
    total=$#
    num=1

    echo "1..$total"
    for defconfig in "$@"; do
	fn=$(basename "$defconfig")
	if disabled_root_login "$defconfig"; then
	    echo "ok $num - $fn"
	else
	    echo "not ok $num - $fn has not disabled root login"
	fi
	num=$((num + 1))
    done
}

check "$CONFIGS"/* || exit 1

exit 0
