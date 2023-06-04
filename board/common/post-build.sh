#!/bin/sh
. "$BR2_CONFIG"
. "$TARGET_DIR/usr/lib/os-release"
if [ -n "${ID_LIKE}" ]; then
    ID="${ID} ${ID_LIKE}"
fi

GIT_VERSION=$(git -C $BR2_EXTERNAL_INFIX_PATH describe --always --dirty --tags)

# This is a symlink to /usr/lib/os-release, so we remove this to keep
# original Buildroot information.
rm "$TARGET_DIR/etc/os-release"
{
    echo "NAME=\"Infix\""
    echo "VERSION=${GIT_VERSION}"
    echo "ID=infix"
    echo "ID_LIKE=\"${ID}\""
    echo "VERSION_ID=${GIT_VERSION}"
    echo "BUILD_ID=\"${NAME} ${VERSION}\""
    echo "PRETTY_NAME=\"Infix by KernelKit\""
    if [ "$INFIX_VARIANT_NETCONF" = "y" ]; then
	echo "VARIANT=\"Managed NETCONF\""
	echo "VARIANT_ID=netconf"
    else
	echo "VARIANT=\"Classic, writable /etc\""
	echo "VARIANT_ID=classic"
    fi
    echo "ARCHITECTURE=\"${INFIX_ARCH}\""
    echo "HOME_URL=https://github.com/KernelKit"
} > "$TARGET_DIR/etc/os-release"


echo "Infix by KernelKit $GIT_VERSION -- $(date +"%b %e %H:%M %Z %Y")" > "$TARGET_DIR/etc/version"

# Allow pdmenu (setup) and bash to be a login shells, bash
# is added automatically when selected in menuyconfig, but
# not when BusyBox provides a symlink (for ash).
grep -qsE '^/usr/bin/pdmenu$$' "$TARGET_DIR/etc/shells" \
        || echo "/usr/bin/pdmenu" >> "$TARGET_DIR/etc/shells"
grep -qsE '^/bin/bash$$' "$TARGET_DIR/etc/shells" \
        || echo "/bin/bash" >> "$TARGET_DIR/etc/shells"

# Menuconfig support for modifying Qemu args in release tarballs
cp "$BR2_EXTERNAL_INFIX_PATH/board/common/qemu/qemu.sh" "$BINARIES_DIR/"
sed "s/default QEMU_aarch64/default QEMU_$BR2_ARCH/" \
    < "$BR2_EXTERNAL_INFIX_PATH/board/common/qemu/Config.in" \
    > "$BINARIES_DIR/Config.in"
rm -f "$BINARIES_DIR/qemu.cfg"
CONFIG_="CONFIG_" BR2_CONFIG="$BINARIES_DIR/qemu.cfg" \
       "$O/build/buildroot-config/conf" --olddefconfig "$BINARIES_DIR/Config.in"
rm -f "$BINARIES_DIR/qemu.cfg.old" "$BINARIES_DIR/.config.old"
