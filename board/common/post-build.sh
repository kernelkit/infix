#!/usr/bin/env bash
# shellcheck disable=SC1090,SC1091,SC3053

common=$(dirname "$(readlink -f "$0")")
. "$common/lib.sh"
. "$BR2_CONFIG" 2>/dev/null
. "$TARGET_DIR/usr/lib/os-release"

# Extract list of loaded YANG modules and their features for yangdoc.html
mkyangdoc()
{
    cmd="yangdoc -o $1 -p $TARGET_DIR/usr/share/yang"

    # shellcheck disable=SC2155
    export SYSREPO_SHM_PREFIX="yangdoc"
    while IFS= read -r line; do
        if echo "$line" | grep -q '^[a-z]'; then
            module=$(echo "$line" | awk '{print $1}')
            cmd="$cmd -m $module"
            feature=$(echo "$line" | awk -F'|' '{print $8}' | sed 's/^ *//;s/ *$//')
            if [ -n "$feature" ]; then
                feature_list=$(echo "$feature" | tr ' ' '\n')
                for feat in $feature_list; do
                    cmd="$cmd -e $feat"
                done
            fi
        fi
    done <<EOF
$(sysrepoctl -l; rm -f /dev/shm/${SYSREPO_SHM_PREFIX}*)
EOF

    # Ignore a few top-level oddballs not used by core Infix
    cmd="$cmd -x supported-algorithms"

    ixmsg "Calling $cmd"
    $cmd
}

if [ -f "$TARGET_DIR/etc/rauc/system.conf" ]; then
    sed -i "s/compatible=.*/compatible=$INFIX_COMPATIBLE/" "$TARGET_DIR/etc/rauc/system.conf"
fi

if [ -n "${ID_LIKE}" ]; then
    ID="${ID} ${ID_LIKE}"
fi

# This is a symlink to /usr/lib/os-release, so we remove this to keep
# original Buildroot information.
ixmsg "Creating /etc/os-release"
rm -f "$TARGET_DIR/etc/os-release"
{
    echo "NAME=\"$INFIX_NAME\""
    echo "ID=$INFIX_ID"
    echo "PRETTY_NAME=\"$INFIX_TAGLINE $INFIX_VERSION\""
    echo "ID_LIKE=\"${ID}\""
    echo "DEFAULT_HOSTNAME=$BR2_TARGET_GENERIC_HOSTNAME"
    echo "VERSION=\"${INFIX_VERSION}\""
    echo "VERSION_ID=${INFIX_VERSION}"
    echo "BUILD_ID=\"${INFIX_BUILD_ID}\""
    if [ -n "$INFIX_IMAGE_ID" ]; then
	echo "IMAGE_ID=\"$INFIX_IMAGE_ID\""
    fi
    if [ -n "$INFIX_RELEASE" ]; then
	echo "IMAGE_VERSION=\"$INFIX_RELEASE\""
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

echo "$INFIX_TAGLINE $INFIX_VERSION -- $(date +"%b %e %H:%M %Z %Y")" > "$TARGET_DIR/etc/version"
ixmsg "Creating /etc/version: $(cat "$TARGET_DIR/etc/version")"

# In case of ambguities, this is what the image was built from
cp "$BR2_CONFIG" "$TARGET_DIR/usr/share/infix/config"
gzip -f "$TARGET_DIR/usr/share/infix/config"

# Drop Buildroot default symlink to /tmp
if [ -L "$TARGET_DIR/var/lib/avahi-autoipd" ]; then
	rm    "$TARGET_DIR/var/lib/avahi-autoipd"
	mkdir "$TARGET_DIR/var/lib/avahi-autoipd"
fi

# Drop Buildroot default pam_lastlog.so from login chain
sed -i '/^[^#]*pam_lastlog.so/s/^/# /' "$TARGET_DIR/etc/pam.d/login"

# Allow bash to be login shells, it is added automatically when selected
# in menuyconfig, but not when BusyBox provides a symlink (for ash).
# The /bin/{true,false} are old UNIX beart means of disabling a user.
grep -qsE '^/bin/bash$$' "$TARGET_DIR/etc/shells" \
        || echo "/bin/bash" >> "$TARGET_DIR/etc/shells"
grep -qsE '^/bin/true$$' "$TARGET_DIR/etc/shells" \
        || echo "/bin/true" >> "$TARGET_DIR/etc/shells"
grep -qsE '^/bin/false$$' "$TARGET_DIR/etc/shells" \
        || echo "/bin/false" >> "$TARGET_DIR/etc/shells"

# Allow clish (symlink to /usr/bin/klish) to be a login shell
grep -qsE '^/bin/clish$$' "$TARGET_DIR/etc/shells" \
        || echo "/bin/clish" >> "$TARGET_DIR/etc/shells"

# Create YANG documentation
if [ "$BR2_PACKAGE_HOST_PYTHON_YANGDOC" = "y" ]; then
   mkyangdoc "$BINARIES_DIR/yangdoc.html"
fi
