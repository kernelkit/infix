#!/bin/sh
# shellcheck disable=SC1090,SC1091
. "$BR2_CONFIG" 2>/dev/null
. "$TARGET_DIR/usr/lib/os-release"

if [ -n "${ID_LIKE}" ]; then
    ID="${ID} ${ID_LIKE}"
fi

if [ -z "$GIT_VERSION" ]; then
    infix_path="$BR2_EXTERNAL_INFIX_PATH"
    if [ -n "$INFIX_OEM_PATH" ]; then
	# Use version from br2-external OEM:ing Infix
	infix_path="$INFIX_OEM_PATH"
    fi
    GIT_VERSION=$(git -C "$infix_path" describe --always --dirty --tags)
fi

# Override VERSION in /etc/os-release and filenames for release builds
if [ -n "$INFIX_RELEASE" ]; then
    VERSION="$INFIX_RELEASE"
else
    VERSION=$GIT_VERSION
fi

# This is a symlink to /usr/lib/os-release, so we remove this to keep
# original Buildroot information.
rm -f "$TARGET_DIR/etc/os-release"
{
    echo "NAME=\"$INFIX_NAME\""
    echo "ID=$INFIX_ID"
    echo "PRETTY_NAME=\"$INFIX_TAGLINE\""
    echo "ID_LIKE=\"${ID}\""
    echo "VERSION=\"${VERSION}\""
    echo "VERSION_ID=${VERSION}"
    echo "BUILD_ID=\"${GIT_VERSION}\""
    if [ -n "$INFIX_IMAGE_ID" ]; then
	echo "IMAGE_ID=\"$INFIX_IMAGE_ID\""
    fi
    if [ -n "$INFIX_RELEASE" ]; then
	echo "IMAGE_VERSION=\"$INFIX_RELEASE\""
    fi
    if [ "$INFIX_VARIANT_NETCONF" = "y" ]; then
	echo "VARIANT=\"Managed NETCONF\""
	echo "VARIANT_ID=netconf"
    else
	echo "VARIANT=\"Classic, writable /etc\""
	echo "VARIANT_ID=classic"
    fi
    echo "ARCHITECTURE=\"${INFIX_ARCH}\""
    echo "HOME_URL=$INFIX_HOME"
    if [ -n "$INFIX_VENDOR" ]; then
	echo "VENDOR_NAME=\"$INFIX_VENDOR\""
    fi
    if [ -n "$INFIX_VENDOR_HOME" ]; then
	echo "VENDOR_HOME=\"$INFIX_VENDOR_HOME\""
    fi
    if [ -n "$INFIX_DOC" ]; then
	echo "DOCUMENTATION_URL=\"$INFIX_DOC\""
    fi
    if [ -n "$INFIX_SUPPORT" ]; then
	echo "SUPPORT_URL=\"$INFIX_SUPPORT\""
    fi
    if [ -n "$INFIX_DESC" ]; then
	echo "INFIX_DESC=\"$INFIX_DESC\""
    fi
} > "$TARGET_DIR/etc/os-release"

echo "$INFIX_TAGLINE $VERSION -- $(date +"%b %e %H:%M %Z %Y")" > "$TARGET_DIR/etc/version"

# Allow pdmenu (setup) and bash to be login shells, bash is added
# automatically when selected in menuyconfig, but not when BusyBox
# provides a symlink (for ash).  The /bin/{true,false} are old UNIX
# beart means of disabling a user.
grep -qsE '^/usr/bin/pdmenu$$' "$TARGET_DIR/etc/shells" \
        || echo "/usr/bin/pdmenu" >> "$TARGET_DIR/etc/shells"
grep -qsE '^/bin/bash$$' "$TARGET_DIR/etc/shells" \
        || echo "/bin/bash" >> "$TARGET_DIR/etc/shells"
grep -qsE '^/bin/true$$' "$TARGET_DIR/etc/shells" \
        || echo "/bin/true" >> "$TARGET_DIR/etc/shells"
grep -qsE '^/bin/false$$' "$TARGET_DIR/etc/shells" \
        || echo "/bin/false" >> "$TARGET_DIR/etc/shells"
