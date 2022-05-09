#!/bin/sh

# shellcheck disable=SC1090
. "$BR2_CONFIG" 2>/dev/null

cat <<-EOF >"$BINARIES_DIR/qemu.sh"
	#!/bin/sh
	line=\$(stty -g)
	stty raw
	qemu-system-x86_64 -M pc -cpu kvm64 -enable-kvm -nographic		\\
	    -kernel bzImage -append "rootwait root=/dev/vda console=ttyS0"	\\
	    -drive file=rootfs.ext2,if=virtio,format=raw			\\
	    -net nic,model=virtio -net user
	stty "\$line"
EOF
chmod +x "$BINARIES_DIR/qemu.sh"
