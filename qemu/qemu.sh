#!/bin/sh

die()
{
    echo "$@" >&2
    exit 1
}

load_qemucfg()
{
    local tmp=$(mktemp -p /tmp)

    grep ^QEMU_ $1 >$tmp
    .  $tmp
    rm $tmp

    [ "$QEMU_MACHINE" ] || die "Missing QEMU_MACHINE"
    [ "$QEMU_KERNEL"  ] || die "Missing QEMU_KERNEL"
    [ "$QEMU_ROOTFS"  ] || die "Missing QEMU_ROOTFS"
    [ "$QEMU_CONSOLE" ] || die "Missing QEMU_CONSOLE"
}

append_args()
{
    echo -n "console=$QEMU_CONSOLE "

    if [ "$QEMU_ROOTFS_INITRD" = "y" ]; then
	# Size of initrd, rounded up to nearest kb
	local size=$((($(stat -c %s $QEMU_ROOTFS) + 1023) >> 10))
	echo -n "root=/dev/ram ramdisk_size=${size} "
    elif [ "$QEMU_ROOTFS_VSCSI" = "y" ]; then
	echo -n "root=/dev/vda "
    fi

    if [ "$V" != "1" ]; then
	echo -n "quiet "
    else
	echo -n "debug "
    fi

    echo -n "${QEMU_APPEND} "
}

rootfs_args()
{
    if [ "$QEMU_ROOTFS_INITRD" = "y" ]; then
	echo -n "-initrd $QEMU_ROOTFS "
    elif [ "$QEMU_ROOTFS_VSCSI" = "y" ]; then
	echo -n "-drive file=$QEMU_ROOTFS,if=virtio,format=raw,bus=0,unit=0 "
    fi
}

rw_args()
{
    [ "$QEMU_RW" ] || return

    if ! [ -f $QEMU_RW ]; then
	dd if=/dev/zero of=$QEMU_RW bs=16M count=1
	mkfs.ext4 -L infix-rw $QEMU_RW
    fi

    echo -n "-drive file=$QEMU_RW,if=virtio,format=raw,bus=0,unit=1 "
}

net_args()
{
    QEMU_NET_MODEL=${QEMU_NET_MODEL:-virtio}

    if [ "$QEMU_NET_BRIDGE" = "y" ]; then
	QEMU_NET_BRIDGE_DEV=${QEMU_NET_BRIDGE_DEV:-virbr0}
	echo -n "-nic bridge,br=$QEMU_NET_BRIDGE_DEV,model=$QEMU_NET_MODEL "
    elif [ "$QEMU_NET_TAP" = "y" ]; then
	QEMU_NET_TAP_N=${QEMU_NET_TAP_N:-1}
	for i in $(seq 0 $(($QEMU_NET_TAP_N - 1))); do
	    echo -n "-nic tap,ifname=qtap$i,script=no,model=$QEMU_NET_MODEL "
	done
    elif [ "$QEMU_NET_USER" = "y" ]; then
	echo -n "-nic user,model=$QEMU_NET_MODEL "
    else
	echo -n "-nic none"
    fi
}

if [ "$1" ]; then
    [ -d "$1" ] || die "Usage: qemu.sh <build-dir>"
    cd $1
fi

load_qemucfg .config

$QEMU_MACHINE -nographic \
	      -kernel $QEMU_KERNEL \
	      $(rootfs_args) \
	      $(rw_args) \
	      $(net_args) \
	      -append "$(append_args)" \
	      $QEMU_EXTRA



