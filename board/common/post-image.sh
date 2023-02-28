#!/bin/sh
# shellcheck disable=SC1090
. "$BR2_CONFIG" 2>/dev/null

common=$(dirname "$(readlink -f "$0")")

# Temporary, separate handling of aarch64 and amd64 images.
# Best would be to have the same for both, i.e., boot GNS3
# with u-boot.
if [ "$BR2_ARCH" = "aarch64" ]; then
    # shellcheck disable=SC2034
    imgdir=$1
    arch=$2
    signkey=$3

    $common/sign.sh $arch $signkey
    $common/mkfit.sh
    $common/mkmmc.sh
elif [ "$BR2_ARCH" = "x86_64" ]; then
    "$common/mkgns3a.sh"
fi
