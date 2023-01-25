#!/bin/sh

GIT_VERSION=$(git -C $BR2_EXTERNAL_INFIX_PATH describe --always --dirty --tags)

# This is a symlink to /usr/lib/os-release, so we remove this to keep
# original Buildroot information.
rm "$TARGET_DIR/etc/os-release"
{
	echo "NAME=\"Infix\""
	echo "VERSION=${GIT_VERSION}"
	echo "ID=infix"
	echo "VERSION_ID=${GIT_VERSION}"
	echo "PRETTY_NAME=\"Infix by KernelKit\""
	echo "HOME_URL=https://github.com/KernelKit"
} > "$TARGET_DIR/etc/os-release"

echo "Infix by KernelKit $GIT_VERSION -- $(date +"%b %e %H:%M %Z %Y")" > "$TARGET_DIR/etc/version"

# Prevent regen of host key at every boot, /etc is saved across reboots
if [ -L "$TARGET_DIR/etc/dropbear" ]; then
	rm    "$TARGET_DIR/etc/dropbear"
	mkdir "$TARGET_DIR/etc/dropbear"
fi

if [ -f "$TARGET_DIR/etc/dhcpcd.conf" ]; then
	if grep -vq background  "$TARGET_DIR/etc/dhcpcd.conf"; then
		cat <<-EOF >> "$TARGET_DIR/etc/dhcpcd.conf"
		
		# Background immediately, do not wait for DHCP lease (speed up boot process)
		background
		
		EOF
	fi
fi
