#!/bin/sh
# shellcheck disable=SC1090
. "$BR2_CONFIG" 2>/dev/null

# Drop Buildroot default symlink to /tmp
if [ -L "$TARGET_DIR/var/lib/avahi-autoipd" ]; then
	rm    "$TARGET_DIR/var/lib/avahi-autoipd"
	mkdir "$TARGET_DIR/var/lib/avahi-autoipd"
fi
