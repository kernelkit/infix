#!/bin/sh
# shellcheck disable=SC2086

common=$(dirname "$(readlink -f "$0")")
. "$common/lib.sh"

# shellcheck disable=SC1091
. "$TARGET_DIR/etc/os-release"

# The INFIX_* variables may be composed from BR2_* variables,
# so we source them last.
load_cfg BR2_ARCH
load_cfg BR2_DEFCONFIG
load_cfg BR2_EXTERNAL_INFIX_PATH
load_cfg BR2_TARGET_ROOTFS
load_cfg INFIX_ID

# The default IMAGE_ID is infix-$BR2_ARCH but can be overridden
# for imaage names, and compat strings, like infix-r2s
if [ -n "$IMAGE_ID" ]; then
    NAME="$IMAGE_ID"
else
    NAME="$INFIX_ID"-$(echo "$BR2_ARCH" | tr _ - | sed 's/x86-64/x86_64/')
fi

ver()
{
    if [ -n "$INFIX_RELEASE" ]; then
	printf -- "-%s" "${INFIX_RELEASE#v}"
	return
    fi
}

diskimg="${NAME}$(ver).qcow2"

# Only for regular builds, not bootloader-only builds
if [ "$BR2_TARGET_ROOTFS_SQUASHFS" = "y" ]; then
    rel=$(ver)
    ln -sf rootfs.squashfs "$BINARIES_DIR/${NAME}${rel}.img"
    if [ -n "$rel" ]; then
	ln -sf "${NAME}${rel}.img" "$BINARIES_DIR/${NAME}.img"
    fi

    cp "$BR2_EXTERNAL_INFIX_PATH/board/common/rootfs/usr/bin/onieprom" "$BINARIES_DIR/"

    # Quick intro for beginners, with links to more information
    cp "$BR2_EXTERNAL_INFIX_PATH/board/common/README.txt" "$BINARIES_DIR/"
fi
