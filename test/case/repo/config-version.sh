#!/bin/sh
# Verify all static .cfg files are at the current confd version

SCRIPT_PATH="$(dirname "$(readlink -f "$0")")"
TOPDIR="$SCRIPT_PATH/../../.."

confd=$(sed -n 's/^AC_INIT(\[confd\], *\[\([^]]*\)\].*/\1/p' "$TOPDIR/src/confd/configure.ac")

version()
{
    jq -r '.["infix-meta:meta"].version' "$1"
}

check()
{
    num=1

    echo "1..$#"
    for cfg in "$@"; do
	name=${cfg#"$TOPDIR"/}
	ver=$(version "$cfg")
	if [ "$ver" = "$confd" ]; then
	    echo "ok $num - $name is at confd version $confd"
	else
	    echo "not ok $num - Unexpected confd version $ver in $name"
	fi
	num=$((num + 1))
    done
    echo "# Configurations can be automatically upgraded using 'make migrate-configs'"
}

# shellcheck disable=SC2046
check $(find "$TOPDIR/board" -name '*.cfg' | xargs grep -l '"infix-meta:meta"' | LC_ALL=C sort)

exit 0
