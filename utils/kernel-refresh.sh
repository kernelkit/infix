#!/bin/bash

usage()
{
    cat <<EOF
usage: $0 -k <kernel-dir> -o <old-version> [-t <kernel-tag>] [-p <patch-dir>] [-d <defconfig-dir>]

Synchronize patches directory with changes from a GIT versioned kernel
tree.

  -k <kernel-dir>    Path to kernel tree.
  -o <old-version>   Version to be replaced by new kernel version.
  -t <kernel-tag>    Base tag from which to generate patches. Default: "v"
                     followed by the value of BR2_LINUX_KERNEL_VERSION in
                     the current configuration.
  -p <patch-dir>     Path to kernel patches directory. Default: the value of
                     BR2_EXTERNAL_INFIX_PATH in the current configuration,
                     followed by "/patches/linux/<kernel-tag>".
  -d <defconfig-dir> Path to defconfig. Default the value of BR2_EXTERNAL_INFIX_PATH
                     in the current configuration, followed by "/configs".

EOF
}

getconfig()
{
    [ -f .config ] || return 1

    grep "$1=" .config | sed -e "s/$1=\"\([^\"]\+\)\"/\1/"
}

if [ $# -lt 1 ]; then
    usage
    exit 1
fi

while getopts "k:o:p:d:t:" flag; do
    case "${flag}" in
            k) KERNEL_DIR=${OPTARG};;
            t) KERNEL_TAG=${OPTARG};;
            o) OLD_VER=${OPTARG};;
            p) PATCH_DIR=${OPTARG};;
            d) DEFCONFIG_DIR=${OPTARG};;
        *) exit 1;;
    esac
done

if ! [ "$KERNEL_TAG" ]; then
    ixkver=$(getconfig BR2_LINUX_KERNEL_VERSION)
    [ "$ixkver" ] && KERNEL_TAG="v$ixkver"
else
    ixkver=${KERNEL_TAG#v}
fi

if ! [ "$KERNEL_TAG" ]; then
    echo "Kernel tag was not supplied, and could not be inferred" >&2
    usage
    exit 1
fi

NEW_VER=${KERNEL_TAG#v}

if ! [ "$PATCH_DIR" ]; then
    ixdir=$(getconfig BR2_EXTERNAL_INFIX_PATH)
    [ "$ixdir" ] && PATCH_DIR="$ixdir/patches/linux/$NEW_VER"
fi

if ! [ "$PATCH_DIR" ]; then
    echo "Patch directory was not supplied, and could not be inferred" >&2
    usage
    exit 1
fi

if ! [ "$DEFCONFIG_DIR" ]; then
    ixdir=$(getconfig BR2_EXTERNAL_INFIX_PATH)
    DEFCONFIG_DIR=$ixdir/configs
fi

KERNEL_DIR=$(readlink -f $KERNEL_DIR)
PATCH_DIR=$(readlink -f $PATCH_DIR)
DEFCONFIG_DIR=$(readlink -f $DEFCONFIG_DIR)

git ls-files --error-unmatch $PATCH_DIR 1>/dev/null 2>&1 && git -C $PATCH_DIR rm *.patch
git -C $KERNEL_DIR format-patch -o $PATCH_DIR $KERNEL_TAG..HEAD
git -C $PATCH_DIR add *.patch
find "$DEFCONFIG_DIR" -name "*_defconfig" -exec sed -i "s/BR2_LINUX_KERNEL_CUSTOM_VERSION_VALUE=\"$OLD_VER\"/BR2_LINUX_KERNEL_CUSTOM_VERSION_VALUE=\"$NEW_VER\"/" {} \;
