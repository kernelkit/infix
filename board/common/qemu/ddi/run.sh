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
#    ./run.sh -c
#
# To bring up a menuconfig dialog.  Select `Exit` and save the changes.
# For more help, see:_
#
#    ./run.sh -h
#
# shellcheck disable=SC3037

# Add /sbin to PATH for mkfs.ext4 and such (not default in debian)
export PATH="/sbin:/usr/sbin:$PATH"

qdir=$(dirname "$(readlink -f "$0")")
imgdir=$(readlink -f "${qdir}/..")
prognm=$(basename "$0")

usage()
{
    cat <<EOF
usage: $prognm [OPTIONS] [-- KERNEL-ARGS]

Start Infix in a VM.

Any arguments after -- are passed to the kernel, provided that QEMU's
native loader is being used (i.e., not UEFI, for example).  This
includes a second --, which the kernel uses to delimit between kernel
arguments and those passed to the init process.

Options:
  -0
    Clear any copy-on-write layers, resetting all disks to their
    initial states, and exit.

  -c
    Run menuconfig to change Qemu settings.  This requires the
    'kconfig-frontends' package (Debian/Ubuntu).

  -G
    Skip config generation. This is useful if you are testing out new
    QEMU features by manually modifying qemu.cfg.

  -h
    Show this help message

Example:

  $prognm -- loglevel=4 -- finit.debug

  Will set the kernel's loglevel to 4, and pass finit.debug as an
  argument to PID 1.

EOF
}

die()
{
    echo "$prognm: $*" >&2
    exit 1
}

binpath()
{
    case "$1" in
	./*|../*)
	    # Relative paths are relative to the location of .config
	    printf "$qdir/$1"
	    ;;
	*)
	    printf "$1"
	    ;;
    esac
}

q()
{
    local _cmd="$1"
    shift

    case "$_cmd" in
	sect)
	    printf ' \\\n\t' >>"$qdir"/qemu.sh

	    case $# in
		1)
		    printf -- "-$1" >>"$qdir"/qemu.sh
		    _delim=" "
		    ;;
		2)
		    printf -- "-$1 $2" >>"$qdir"/qemu.sh
		    _delim=","
		    ;;
		*)
		    die "q sect: Invalid arguments"
		    ;;
	    esac
	    ;;
	param)
	    [ $# -eq 2 ] || die "q param: Takes exactly two arguments"
	    printf -- "$_delim$1=$2" >>"$qdir"/qemu.sh
	    _delim=","
	    ;;
	option)
	    [ $# -eq 1 ] || die "q option: Takes exactly one argument"
	    printf -- "$_delim$1" >>"$qdir"/qemu.sh
	    _delim=","
	    ;;
	*)
	    die "q: Unknown command: $_cmd"
	    ;;
    esac
}

q_sect_device_virtio()
{
    q sect device virtio-$1-$QEMU_VIRTIO_BUS
}

q_add_disk()
{
    if [ "$QEMU_DISK_SYS_IF_VIRTIO" ]; then
    	q_sect_device_virtio blk
	q param drive "$1"
    else
	die "Unknown system disk interface"
    fi
}

q_sect_device_usb()
{
    if [ -z "$usb_bus_added" ]; then
	q sect usb
	q sect device usb-ehci
	q param id ehci
	usb_bus_added=YES
    fi

    q sect device "$1"
    q param bus ehci.0
}

qcowed()
{
    local _qcow="$qdir"/"$1".qcow2
    local _base _size

    if [ -f "$_qcow" ] && qemu-img check -q "$_qcow"; then
	echo "$_qcow"
	return
    fi

    rm -f "$_qcow"

    case $# in
	2)
	    _size="$2"

	    qemu-img create -q -f qcow2 "$_qcow" "$_size" \
		|| die "Unable to create $1"
	    ;;
	3)

	    _base="$2"
	    _size="$3"

	    [ "$_size" = "auto" ] && _size=

	    qemu-img create -q -f qcow2 -F raw \
		     -o backing_file="$_base" "$_qcow" $_size \
		|| die "Unable to create CoW layer for $_base"
	    ;;
	*)
	    die "qcowed: usage: qcowed <name> [<backing-file>] <size>"
	    ;;
    esac

    echo "$_qcow"
}

append()
{
    echo -n " $*" >>"$qdir"/append
}

gen_machine()
{
    q sect machine
    q param type  "$QEMU_MACHINE"
    q param accel kvm:tcg

    q sect cpu "$QEMU_CPU"

    q sect m
    q param size "$QEMU_RAM"
}

gen_loader()
{
    if [ "$QEMU_LOADER_QEMU" ]; then
	q sect kernel $(binpath "$QEMU_BIN_KERNEL")
    elif [ "$QEMU_LOADER_OVMF" ]; then
	q sect bios $(binpath "$QEMU_BIN_BIOS")
    fi
}

gen_serial()
{
    q sect display none

    q_sect_device_virtio serial

    q sect chardev stdio
    q param id  console0
    q param mux on

    q sect mon
    q param chardev console0

    case "$QEMU_CONSOLE" in
	hvc0)
	    q sect device virtconsole
	    q param nr      0
	    q param name    console
	    q param chardev console0
	    ;;
	ttyS0|ttyAMA0)
	    q sect serial chardev:console0
	    ;;
	*)
	    die "Unsupported console: $QEMU_CONSOLE"
	    ;;
    esac

    append console="$QEMU_CONSOLE"

    [ "$V" ] && append debug || append loglevel=4
}

gpt_uuid()
{
    sgdisk "$1" -p | awk '
    	BEGIN { err = 1; } END { exit(err); }
	/^Creating new GPT/ { exit; }

	/^Disk identifier \(GUID\): / {
	    print($4); err = 0; exit;
	}' || die "Unable to determine GPT UUID of $1"
}

gen_disk_sys_empty()
{
    q sect drive
    q param id        disk-sys
    q param format    qcow2
    q param if        none
    q param file      $(qcowed disk-sys "$QEMU_DISK_SYS_SIZE")

    q_add_disk disk-sys
}

gen_disk_sys_full()
{
    q sect drive
    q param id        disk-sys
    q param format    qcow2
    q param if        none
    q param file      $(qcowed disk-sys $(binpath "$QEMU_BIN_DISK") \
			       "$QEMU_DISK_SYS_SIZE")

    q_add_disk disk-sys

    append root=ddi:lvm:internal/$(basename "$QEMU_BIN_DDI")
}

gen_disk_sys_split()
{
    # EFI System Partition
    q sect drive
    q param id        disk-esp
    q param format    qcow2
    q param if        none
    q param file      $(qcowed disk-esp $(binpath "$QEMU_BIN_ESP") auto)

    q_add_disk disk-esp

    # LVM stub
    q sect drive
    q param id        disk-sys
    q param format    qcow2
    q param if        none
    q param file      $(qcowed disk-sys $(binpath "$QEMU_BIN_LVM_STUB") \
			       "$QEMU_DISK_SYS_SIZE")

    q_add_disk disk-sys

    # DDI (ro)
    q sect drive
    q param id        disk-ddi
    q param format    raw
    q param file      $(binpath "$QEMU_BIN_DDI")
    q param if        none
    q param read-only on

    q_add_disk disk-ddi

    append root=ddi:GPTUUID=$(gpt_uuid $(binpath "$QEMU_BIN_DDI"))
}

gen_disk_sys()
{
    [ "$QEMU_DISK_SYS_NONE" ] && return

    if [ "$QEMU_DISK_SYS_EMPTY" ]; then
	gen_disk_sys_empty
    elif [ "$QEMU_DISK_SYS_FULL" ]; then
	gen_disk_sys_full
    elif [ "$QEMU_DISK_SYS_SPLIT" ]; then
	gen_disk_sys_split
    else
	die "Unknown system disk mode"
    fi
}

gen_disk_usb()
{
    [ "$QEMU_DISK_USB_NONE" ] && return

    if [ "$QEMU_DISK_USB_DISK" ]; then
	q sect drive
	q param id        disk-usb
	q param format    qcow2
	q param file      $(qcowed disk-usb \
				$(binpath "$QEMU_BIN_DISK") \
				"$QEMU_DISK_USB_SIZE")
	q param if        none

	q_sect_device_usb usb-storage
	q param drive disk-usb
    else
	die "Unknown USB attachment"
    fi
}

gen_disk_initrd()
{
    [ "$QEMU_DISK_INITRD" ] || return

    q sect initrd $(binpath "$QEMU_BIN_DDI")
    append root=ddi:/initrd.image
}

internal_is_available()
{
    [ "$QEMU_DISK_SYS_FULL" ] || [ "$QEMU_DISK_SYS_SPLIT" ] \
	|| [ "$QEMU_DISK_USB_DISK" ]
}

gen_disks()
{
    gen_disk_sys
    gen_disk_usb

    gen_disk_initrd

    internal_is_available || append rd.nointernal
}

gen_gdb()
{
    # Create a UNIX socket on the host that is connected to a virtio
    # console in the guest, which gdbserver can attach to for
    # userspace debugging.
    q sect chardev socket
    q param id     gdbserver
    q param path   "$qdir"/gdbserver.sock
    q param server on
    q param wait   off

    q sect device virtconsole
    q param nr      1
    q param name    gdbserver
    q param chardev gdbserver

    # Create a UNIX socket on the host that is connected to QEMU's GDB
    # stub, for bootloader/kernel debugging.
    q sect chardev socket
    q param id gdbqemu
    q param path   "$qdir"/gdbqemu.sock
    q param server on
    q param wait   off

    q sect gdb chardev:gdbqemu
}

gen_all()
{
    local _append

    : >"$qdir"/append
    cat <<EOF >"$qdir"/qemu.sh
#!/bin/sh

echo "Starting Qemu  ::  Ctrl-a x -- exit | Ctrl-a c -- toggle console/monitor"

line=\$(stty -g)
stty raw
trap 'stty "\$line"' EXIT INT TERM

qemu-system-$QEMU_ARCH -nodefaults \\
EOF
    chmod +x "$qdir"/qemu.sh

    gen_machine
    gen_loader
    gen_serial
    gen_disks
    gen_gdb

    if [ "$QEMU_LOADER_QEMU" ]; then
	_append=$(cat "$qdir"/append)
	q sect append "\"$_append $QEMU_APPEND $*\""
    fi

    echo >>"$qdir"/qemu.sh
}

menuconfig()
{
    command -v kconfig-mconf >/dev/null \
	|| die "cannot find kconfig-mconf for menuconfig"

    CONFIG_= KCONFIG_CONFIG="$qdir"/.config \
	   kconfig-mconf "$qdir"/Config.in
}

_generate=YES

while getopts "0cGh-" opt; do
    case ${opt} in
	0)
	    echo "Clearing all copy-on-write layers" >&2
	    rm -f "$qdir"/*.qcow2
	    exit 0
	    ;;
	c)
	    menuconfig
	    ;;
	G)
	    _generate=
	    ;;
	h)
	    usage && exit 0
	    ;;
	-)
	    break
	    ;;
	*)
	    usage && exit 1
	    ;;
    esac
done
shift $((OPTIND - 1))

# shellcheck disable=SC1090
. "$qdir"/.config

[ "$_generate" ] && gen_all "$*"

exec "$qdir"/qemu.sh
