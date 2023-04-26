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

# Allow pdmenu (setup) to be a login shell
grep -qsE '^/usr/bin/pdmenu$$' "$TARGET_DIR/etc/shells" \
        || echo "/usr/bin/pdmenu" >> "$TARGET_DIR/etc/shells"
