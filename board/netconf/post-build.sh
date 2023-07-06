#!/bin/sh
# shellcheck disable=SC1090
. "$BR2_CONFIG" 2>/dev/null

# Drop Buildroot default symlink to /tmp
if [ -L "$TARGET_DIR/var/lib/avahi-autoipd" ]; then
	rm    "$TARGET_DIR/var/lib/avahi-autoipd"
	mkdir "$TARGET_DIR/var/lib/avahi-autoipd"
fi

# Allow clish (symlink to /usr/bin/klish) to be a login shell
grep -qsE '^//bin/clish$$' "$TARGET_DIR/etc/shells" \
        || echo "/bin/clish" >> "$TARGET_DIR/etc/shells"
