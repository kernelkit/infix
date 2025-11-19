#!/bin/sh

# Only for regular builds, not bootloader-only builds
if [ "$BR2_TARGET_ROOTFS_SQUASHFS" = "y" ]; then
    # Quick intro for beginners, with links to more information
    cp "$BR2_EXTERNAL_INFIX_PATH/board/common/README.txt" "$BINARIES_DIR/"
fi
