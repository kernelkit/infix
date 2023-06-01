#!/bin/sh
# This script can be used to start an Infix OS image in Qemu.  It reads
# either a .config, generated from Config.in, or qemu.cfg from a release
# tarball, for the required configuration data.
#
# Debian/Ubuntu users can change the configuration post-release, install
# the kconfig-frontends package:
#
#    sudo apt install kconfig-frontends
#
# and then call this script with:
#
#    ./qemu.sh -c
#
# To bring up a menuconfig dialog.  Select `Exit` and save the changes.
# For more help, see:_
#
#    ./qemu.sh -h
#

# Local variables
prognm=$(basename "$0")

usage()
{
    echo "usage: $prognm [opts] [buildroot-output-path]"
    echo
    echo " -c    Run menuconfig from release tarball or images diratory"
    echo " -h    This help text"
    echo
    echo "Note: 'kconfig-frontends' must be installed for -c to work."

    exit 0
}

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
    [ "$QEMU_ROOTFS"  ] || die "Missing QEMU_ROOTFS"

    [ "$QEMU_KERNEL" -a "$QEMU_BIOS" ] \
	&& die "QEMU_KERNEL conflicts with QEMU_BIOS"

    [ ! "$QEMU_KERNEL" -a ! "$QEMU_BIOS"  ] \
	&& die "QEMU_KERNEL or QEMU_BIOS must be set"
}

loader_args()
{
    if [ "$QEMU_BIOS" ]; then
	echo -n "-bios $QEMU_BIOS "
    elif [ "$QEMU_KERNEL" ]; then
	echo -n "-kernel $QEMU_KERNEL "
    fi
}

append_args()
{
# Disabled, not needed anymore with virtconsole (hvc0)
#    [ "$QEMU_CONSOLE" ] && echo -n "console=$QEMU_CONSOLE "

    echo -n "console=hvc0 "
    if [ "$QEMU_ROOTFS_INITRD" = "y" ]; then
	# Size of initrd, rounded up to nearest kb
	local size=$((($(stat -c %s $QEMU_ROOTFS) + 1023) >> 10))
	echo -n "root=/dev/ram ramdisk_size=${size} "
    elif [ "$QEMU_ROOTFS_VSCSI" = "y" ]; then
	echo -n "root=PARTLABEL=primary "
    fi

    if [ "$V" != "1" ]; then
	echo -n "quiet "
    else
	echo -n "debug "
    fi

    echo -n "${QEMU_APPEND} ${QEMU_EXTRA_APPEND} "
}

rootfs_args()
{
    if [ "$QEMU_ROOTFS_INITRD" = "y" ]; then
	echo -n "-initrd $QEMU_ROOTFS "
    elif [ "$QEMU_ROOTFS_MMC" = "y" ]; then
	echo -n "-device sdhci-pci "
	echo -n "-device sd-card,drive=mmc "
	echo -n "-drive id=mmc,file=$QEMU_ROOTFS,if=none,format=raw "
    elif [ "$QEMU_ROOTFS_VSCSI" = "y" ]; then
	echo -n "-drive file=$QEMU_ROOTFS,if=virtio,format=raw,bus=0,unit=0 "
    fi
}

rw_args()
{
    [ "$QEMU_RW" ] || return

    if ! [ -f "$QEMU_RW" ]; then
	dd if=/dev/zero of="$QEMU_RW" bs=16M count=1 >/dev/null 2>&1
	mkfs.ext4 -L cfg "$QEMU_RW" >/dev/null 2>&1
    fi

    echo -n "-drive file=$QEMU_RW,if=virtio,format=raw,bus=0,unit=1 "

    if [ "$QEMU_RW_VAR_OPT" ]; then
	if ! [ -f "$QEMU_RW_VAR" ]; then
	    dd if=/dev/zero of="$QEMU_RW_VAR" bs=256M count=1 >/dev/null 2>&1
	    mkfs.ext4 -L var "$QEMU_RW_VAR" >/dev/null 2>&1
	fi

	echo -n "-drive file=$QEMU_RW_VAR,if=virtio,format=raw,bus=0,unit=2 "
    fi
}

host_args()
{
    [ "${QEMU_HOST}" ] || return

    echo -n "-virtfs local,path=${QEMU_HOST},security_model=none,writeout=immediate,mount_tag=hostfs "
}

net_args()
{
    QEMU_NET_MODEL=${QEMU_NET_MODEL:-virtio}

    if [ "$QEMU_NET_BRIDGE" = "y" ]; then
	QEMU_NET_BRIDGE_DEV=${QEMU_NET_BRIDGE_DEV:-virbr0}
	echo -n "-nic bridge,br=$QEMU_NET_BRIDGE_DEV,model=$QEMU_NET_MODEL "
    elif [ "$QEMU_NET_TAP" = "y" ]; then
	QEMU_NET_TAP_N=${QEMU_NET_TAP_N:-1}
	mactab=$(dirname "$QEMU_ROOTFS")/mactab
	rm -f "$mactab"
	for i in $(seq 0 $(($QEMU_NET_TAP_N - 1))); do
	    printf "e$i	52:54:00:12:34:%02x\n" $((0x56 + i)) >>"$mactab"
	    echo -n "-netdev tap,id=nd$i,ifname=qtap$i -device e1000,netdev=nd$i "
	done
	echo -n "-fw_cfg name=opt/mactab,file=$mactab "
    elif [ "$QEMU_NET_USER" = "y" ]; then
	[ "$QEMU_NET_USER_OPTS" ] && QEMU_NET_USER_OPTS="$QEMU_NET_USER_OPTS,"

	echo -n "-nic user,${QEMU_NET_USER_OPTS}model=$QEMU_NET_MODEL "
    else
	echo -n "-nic none"
    fi
}

wdt_args()
{
    echo -n "-device i6300esb -rtc clock=host"
}

run_qemu()
{
    local qemu
    read qemu <<EOF
	$QEMU_MACHINE \
	  -display none -rtc base=utc,clock=vm \
	  -device virtio-serial -chardev stdio,mux=on,id=console0 \
	  -device virtconsole,chardev=console0 -mon chardev=console0 \
	  -chardev socket,id=gdbserver,path=gdbserver.sock,server=on,wait=off \
	  -device virtconsole,name=console1,chardev=gdbserver \
	  $(loader_args) \
	  $(rootfs_args) \
	  $(rw_args) \
	  $(host_args) \
	  $(net_args) \
	  $(wdt_args) \
	  $QEMU_EXTRA
EOF

    if [ "$QEMU_KERNEL" ]; then
	$qemu -append "$(append_args)" "$@"
    else
	$qemu "$@"
    fi
}

dtb_args()
{
    [ "$QEMU_LOADER_UBOOT" ] || return

    if [ "$QEMU_DTB_EXTEND" ]; then
	# On the current architecture, QEMU will generate an internal
	# DT based on the system configuration.

	# So we extract a copy of that
	run_qemu -M dumpdtb=images/qemu.dtb >/dev/null 2>&1

	# Extend it with the environment and signing information in
	# u-boot.dtb.
	echo "images/qemu.dtb images/u-boot.dtb" | \
	    xargs -n 1 dtc -I dtb -O dts | \
	    { echo "/dts-v1/;"; sed  -e 's:/dts-v[0-9]\+/;::'; } | \
	    dtc >images/qemu-extended.dtb 2>/dev/null

	# And use the combined result to start the instance
	echo -n "-dtb images/qemu-extended.dtb "
    else
	# Otherwise we just use the unmodified one
	echo -n "-dtb images/u-boot.dtb "
    fi
}

menuconfig()
{
    command -v kconfig-mconf >/dev/null || die "Cannot find kconfig-mconf, try installing the kconfig-frontends package."

    # Setting CONFIG_ variable cannot be used to drop CONFIG_ prefix so
    # we use sed to drop any such prefix on successful exit
    kconfig-mconf Config.in
    if [ -f .config ]; then
	sed -i 's/CONFIG_//g' .config
	exit 0
    else
	exit 1
    fi
}

while [ "$1" != "" ]; do
    case $1 in
	-c)
	    menuconfig
	    ;;
	-h | *)
	    usage
	    ;;
    esac
    shift
done

if [ "$1" ]; then
    [ -d "$1" ] || usage
    cd "$1" || die "Failed chanding to output directory $1"
fi

# 'make run' from output/ or build directory, but can also be ./qemu.sh
# run from output/images/ or an unpacked release tarball.
if [ -f .config ]; then
    load_qemucfg .config
else
    load_qemucfg qemu.cfg
fi

echo "Starting Qemu  ::  Ctrl-a x -- exit | Ctrl-a c -- toggle console/monitor"
line=$(stty -g)
stty raw
run_qemu $(dtb_args)
stty "$line"

