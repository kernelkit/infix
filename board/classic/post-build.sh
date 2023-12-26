#!/bin/sh
. "$BR2_CONFIG"

# Prevent regen of host key at every boot, /etc is saved across reboots
if [ -L "$TARGET_DIR/etc/dropbear" ]; then
	rm    "$TARGET_DIR/etc/dropbear"
	mkdir "$TARGET_DIR/etc/dropbear"
fi

# Classic builds don't have D-Bus
if [ -f "$TARGET_DIR/etc/dnsmasq.conf" ]; then
    sed -i '/enable-dbus/d' "$TARGET_DIR/etc/dnsmasq.conf"
fi
