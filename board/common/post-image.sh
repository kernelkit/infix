#!/bin/sh
# shellcheck disable=SC1090
. "$BR2_CONFIG" 2>/dev/null

common=$(dirname "$(readlink -f "$0")")

imgdir=$1
arch=$2
signkey=$3

$common/sign.sh $arch $signkey
$common/mkfit.sh
$common/mkmmc.sh

if [ "$BR2_ARCH" = "x86_64" ]; then
	"$common/mkgns3a.sh"
fi
