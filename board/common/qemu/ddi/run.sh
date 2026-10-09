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
    q sect device virtio-$1-$IX_QEMU_VIRTIO_BUS
}

q_add_disk()
{
    if [ "$IX_QEMU_DISK_SYS_IF_VIRTIO" ]; then
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

appendroot()
{
    append root=/dev/mapper/root
    append roothash=$(sfdisk -J $(binpath "$IX_QEMU_BIN_DDI") \
			  | jq -r '.partitiontable.partitions | "\(.[0].uuid)-\(.[1].uuid)"' \
			  | tr -d '-' | tr 'A-F' 'a-f')
}

gen_machine()
{
    q sect machine
    q param type  "$IX_QEMU_MACHINE"
    q param accel kvm:tcg

    q sect cpu "$IX_QEMU_CPU"

    q sect m
    q param size "$IX_QEMU_RAM"
}

gen_pflash_ovmf_code()
{
    q sect drive
    q param id        ovmf-code
    q param format    raw
    q param if        pflash
    q param unit      0
    q param file      $(binpath "$IX_QEMU_BIN_OVMF_CODE")
    q param readonly  on
}

gen_pflash_ovmf_vars()
{
    local _src=$(binpath "$IX_QEMU_BIN_OVMF_VARS")
    local _dst="$qdir"/ovmf-vars-$(grep _OVMF_ "$qdir"/.config | sha256sum | head -c 8).fd
    local _guid=$(uuidgen)

    if ! [ -f "$_dst" ]; then
	[ -f "$_src" ] || die "OVMF Variable template ($_src) does not exist"
	cp "$_src" "$_dst"

	if [ "$IX_QEMU_OVMF_SB" ]; then
	    command virt-fw-vars \
		|| die "Please install virt-fw-vars to support injection of OVMF secure boot variables"

	    virt-fw-vars \
		--inplace "$_dst" \
		--set-pk  $_guid $(binpath "$IX_QEMU_OVMF_SB_PK") \
		--add-kek $_guid $(binpath "$IX_QEMU_OVMF_SB_KEK") \
		--add-db  $_guid $(binpath "$IX_QEMU_OVMF_SB_DB") \
		--secure-boot \
		|| die "Failed to inject secure boot keys to OVMF variable image"
	fi
    fi

    q sect drive
    q param id        ovmf-vars
    q param format    raw
    q param if        pflash
    q param unit      1
    q param file      "$_dst"
}

gen_loader()
{
    if [ "$IX_QEMU_LOADER_QEMU" ]; then
	q sect kernel $(binpath "$IX_QEMU_BIN_KERNEL")
	appendroot
    elif [ "$IX_QEMU_LOADER_OVMF" ]; then
	gen_pflash_ovmf_code
	gen_pflash_ovmf_vars

	if [ "$IX_QEMU_OVMF_SB" ]; then
	    q sect global
	    q param driver   cfi.pflash01
	    q param property secure
	    q param value    on
	fi
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

    case "$IX_QEMU_CONSOLE" in
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
	    die "Unsupported console: $IX_QEMU_CONSOLE"
	    ;;
    esac

    append console="$IX_QEMU_CONSOLE"

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
    q param file      $(qcowed disk-sys "$IX_QEMU_DISK_SYS_SIZE")

    q_add_disk disk-sys
}

gen_disk_sys_full()
{
    q sect drive
    q param id        disk-sys
    q param format    qcow2
    q param if        none
    q param file      $(qcowed disk-sys $(binpath "$IX_QEMU_BIN_DISK") \
			       "$IX_QEMU_DISK_SYS_SIZE")

    q_add_disk disk-sys
}

gen_disk_sys_split()
{
    # LVM stub
    q sect drive
    q param id        disk-stub
    q param format    qcow2
    q param if        none
    q param file      $(qcowed disk-stub $(binpath "$IX_QEMU_BIN_LVM_STUB") \
			       "$IX_QEMU_DISK_SYS_SIZE")

    q_add_disk disk-stub

    # DDI (ro)
    q sect drive
    q param id        disk-ddi
    q param format    raw
    q param file      $(binpath "$IX_QEMU_BIN_DDI")
    q param if        none
    q param read-only on

    q_add_disk disk-ddi
}

gen_disk_sys()
{
    [ "$IX_QEMU_DISK_SYS_NONE" ] && return

    if [ "$IX_QEMU_DISK_SYS_EMPTY" ]; then
	gen_disk_sys_empty
    elif [ "$IX_QEMU_DISK_SYS_FULL" ]; then
	gen_disk_sys_full
    elif [ "$IX_QEMU_DISK_SYS_SPLIT" ]; then
	gen_disk_sys_split
    else
	die "Unknown system disk mode"
    fi
}

gen_disk_usb()
{
    [ "$IX_QEMU_DISK_USB_NONE" ] && return

    if [ "$IX_QEMU_DISK_USB_DISK" ]; then
	q sect drive
	q param id        disk-usb
	q param format    qcow2
	q param file      $(qcowed disk-usb \
				$(binpath "$IX_QEMU_BIN_DISK") \
				"$IX_QEMU_DISK_USB_SIZE")
	q param if        none

	q_sect_device_usb usb-storage
	q param drive disk-usb
    else
	die "Unknown USB attachment"
    fi
}

gen_disk_host()
{
    [ -d "$IX_QEMU_DISK_HOST" ] || return

    q sect virtfs local
    q param path "$IX_QEMU_DISK_HOST"
    q param security_model none
    q param writeout immediate
    q param mount_tag host
}

gen_disk_initrd()
{
    [ "$IX_QEMU_DISK_INITRD" ] || return

    q sect initrd $(binpath "$IX_QEMU_BIN_DDI")
}

internal_is_available()
{
    [ "$IX_QEMU_DISK_SYS_FULL" ] || [ "$IX_QEMU_DISK_SYS_SPLIT" ] \
	|| [ "$IX_QEMU_DISK_USB_DISK" ]
}

gen_disks()
{
    gen_disk_sys
    gen_disk_usb
    gen_disk_host

    gen_disk_initrd

    internal_is_available || append rd.nointernal
}

gen_net_dev()
{
    local _name="$1"
    local _idx="$2"
    local _mac=$(printf "02:00:00:de:ad:%02x" "$_idx")

    echo "$_name $_mac" >>"$qdir"/mactab

    q sect device "$IX_QEMU_NET_MODEL"
    q param netdev "$_name"
    q param mac    "$_mac"
}

gen_net_user()
{
    [ "$IX_QEMU_NET_USER" ] || return

    q sect netdev user
    q param id e1

    if [ "$IX_QEMU_NET_USER_NETBOOT" ]; then
	q param tftp     $(dirname $(readlink -f $(binpath "$IX_QEMU_BIN_DDI")))
	q param bootfile $(basename "$IX_QEMU_BIN_DDI")
    fi

    if [ "$IX_QEMU_NET_USER_OPTS" ]; then
	q option "$IX_QEMU_NET_USER_OPTS"
    fi

    gen_net_dev e1 1
}

gen_net()
{
    :> "$qdir"/mactab
    q sect fw_cfg
    q param name opt/mactab
    q param file "$qdir"/mactab

    gen_net_user
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

    q sect device virtserialport
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

qemu-system-$IX_QEMU_ARCH -nodefaults \\
EOF
    chmod +x "$qdir"/qemu.sh

    gen_machine
    gen_loader
    gen_serial
    gen_disks
    gen_net
    gen_gdb

    if [ "$IX_QEMU_LOADER_QEMU" ]; then
	_append=$(cat "$qdir"/append)
	q sect append "\"$_append $IX_QEMU_APPEND $*\""
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
	    rm -f "$qdir"/ovmf-vars-*.fd
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
