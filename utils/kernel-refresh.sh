#!/bin/bash
set -e

usage()
{
    cat <<EOF
Synchronize patches/linux/\$TAG with changes from a kernel GIT tree.

Usage:
  $0 -k kernel-dir -o old-version [-t kernel-tag] [-p patch-dir] [-d defconfig-dir]

Options:
  -h                 This help text
  -k kernel-dir      Path to kernel tree
  -o old-version     Version to be replaced by new kernel version
  -t kernel-tag      Base tag from which to generate patches.
                     Default: "v" + \$BR2_LINUX_KERNEL_VERSION
  -p patch-dir       Path to kernel patches directory
                     Default: \$BR2_EXTERNAL_INFIX_PATH + "/patches/linux/\$kernel-tag"
  -d defconfig-dir   Path to defconfig
                     Default: \$BR2_EXTERNAL_INFIX_PATH + "/configs"

Example:
  cd infix/output
  ln -s ../local.mk     # Set LINUX_OVERRIDE_SRCDIR to git tree, e.g., ~/src/linux
  ../utils/kernel-refresh.sh -k ~/src/linux -o 6.12.21 -t v6.12.21 \\
                             -p ~/src/x-misc/patches/linux/6.12.21

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

while getopts "hk:o:p:d:t:" flag; do
    case "${flag}" in
	h) usage; exit 0;;
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
PATCHES_BASE=$(dirname $PATCH_DIR)

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

git ls-files --error-unmatch $PATCH_DIR 1>/dev/null 2>&1 && git -C $PATCH_DIR rm -f *.patch
git -C $KERNEL_DIR format-patch --no-signoff --no-encode-email-headers --no-cover-letter --no-signature -o $PATCH_DIR $KERNEL_TAG..HEAD
git -C $PATCH_DIR add *.patch
if [ -d ${PATCHES_BASE}/${OLD_VER} ]; then
	git rm -rf ${PATCHES_BASE}/${OLD_VER}
fi
find "$DEFCONFIG_DIR" -name "*_defconfig" -exec sed -i "s/BR2_LINUX_KERNEL_CUSTOM_VERSION_VALUE=\"$OLD_VER\"/BR2_LINUX_KERNEL_CUSTOM_VERSION_VALUE=\"$NEW_VER\"/" {} \;
git -C $DEFCONFIG_DIR add *_defconfig

echo "Update checksum for kernel, this may take a while..."
curl -o "/tmp/linux-${ixkver}.tar.xz" "https://cdn.kernel.org/pub/linux/kernel/v6.x/linux-${ixkver}.tar.xz"
echo "# Calculated with $0" > "${PATCHES_BASE}/linux.hash"
cd /tmp && echo "sha256   $(sha256sum linux-${ixkver}.tar.xz)" >> "${PATCHES_BASE}/linux.hash"
git -C ${PATCHES_BASE} add linux.hash
