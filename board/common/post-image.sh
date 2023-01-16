#!/bin/sh
# shellcheck disable=SC1090
. "$BR2_CONFIG" 2>/dev/null

common=$(dirname "$(readlink -f "$0")")

"$common/mkfit.sh"

if [ "$BR2_ARCH" = "x86_64" ]; then
	"$common/mkgns3a.sh"
fi
