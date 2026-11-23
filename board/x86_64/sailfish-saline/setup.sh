#!/bin/sh
# Prepare a host to pass a saline board through to an Infix QEMU VM over VFIO

VEND=10ee
PROD_GEN3=9034
PROD_GEN1=9024

vfiodir=/sys/bus/pci/drivers/vfio-pci
rulesfile=99-saline-vfio.rules

err=0

ok()
{
    echo "OK:    $*" >&2
}

warn()
{
    echo "WARN:  $*" >&2
}

fail()
{
    echo "ERROR: $*" >&2
    err=1
}

die()
{
    fail "$@"
    exit 1
}

iommu_detect()
{
    if [ -f /sys/firmware/acpi/tables/DMAR ]; then
	iommu=intel
    else
	iommu=
    fi
}

locate()
{
    dev=$(lspci -nDd $VEND:$PROD_GEN3 | cut -d ' ' -f 1)
    [ "$dev" ] || dev=$(lspci -nDd $VEND:$PROD_GEN1 | cut -d ' ' -f 1)
    [ "$dev" ]
}

check_iommu()
{
    iommu_detect
    case "$iommu" in
	intel)
	    ok "Intel IOMMU found"
	    ;;
	*)
	    fail "No supported IOMMU found (supported: intel)"
	    return 1
	    ;;
    esac

    if [ -n "$(ls /sys/class/iommu 2>/dev/null)" ]; then
	ok "IOMMU is enabled"
    else
	fail "IOMMU is disabled, add intel_iommu=on to the kernel command line"
	return 1
    fi
}

check_device()
{
    if [ "$dev" ]; then
	[ -d "/sys/bus/pci/devices/$dev" ] || {
	    fail "$dev does not seem to be an existing PCI device"
	    return 1
	}
    else
	locate || {
	    fail "Found no saline board ($VEND:$PROD_GEN3 or $VEND:$PROD_GEN1)"
	    return 1
	}
    fi
    devdir="/sys/bus/pci/devices/$dev"
    ok "Saline board found at $dev"

    [ -d "$devdir"/iommu_group ] || {
	fail "$dev is not associated with any IOMMU group"
	return 1
    }
    group=$(basename "$(readlink -f "$devdir"/iommu_group)")
    ok "$dev is in IOMMU group $group"
}

check_vfio()
{
    if [ "$(readlink -f "$devdir"/driver)" = "$(readlink -f $vfiodir)" ]; then
	ok "$dev is bound to vfio-pci"
    else
	fail "$dev is not bound to vfio-pci, install $rulesfile in /etc/udev/rules.d/"
	return 1
    fi

    for node in /dev/vfio/vfio /dev/vfio/"$group"; do
	if [ -r "$node" ] && [ -w "$node" ]; then
	    ok "$node is accessible"
	else
	    fail "$node is not accessible to $(id -un)"
	fi
    done

    memlock=$(ulimit -l)
    if [ "$memlock" = unlimited ]; then
	ok "Locked memory limit is unlimited"
    else
	warn "Locked memory limit is ${memlock} kB, VFIO must lock all VM RAM"
    fi
}

check()
{
    check_iommu
    check_device && check_vfio

    if [ $err -eq 0 ]; then
	echo "$dev is ready for use" >&2
    fi
    return $err
}

configure_intel_iommu()
{
    mv "$1" "$1".old
    awk -vdev="$dev" '
    /^IX_QEMU_MACHINE=/ {
        print("IX_QEMU_MACHINE=\"qemu-system-x86_64 -M q35,accel=kvm,kernel-irqchip=split\"");
	next;
    }
    /^IX_QEMU_EXTRA=/ {
        printf("IX_QEMU_EXTRA=\"-device intel-iommu,intremap=on,caching-mode=on,aw-bits=39 -device vfio-pci,host=%s\"\n", dev);
	next;
    }
    { print; }
    ' "$1".old >"$1"
}

configure()
{
    [ -f "$1" ] || die "No such file: $1"

    check_device || exit 1

    iommu_detect
    case "$iommu" in
	intel)
	    configure_intel_iommu "$1"
	    ;;
	*)
	    die "No supported IOMMU found (supported: intel)"
	    ;;
    esac

    echo "VM started from $1 will now attach to $dev" >&2
}

usage()
{
    cat <<EOF
Usage: $0 check [DEV]           -- Verify that the host is ready to pass
                                   a saline board through to a VM
       $0 configure [DEV] FILE  -- Inject saline QEMU options in .config file

       If DEV is supplied it should be the full PCI address (including
       domain) of the saline board to operate on. If omitted, the
       first device matching $VEND:$PROD_GEN3 or $VEND:$PROD_GEN1 is used.
EOF
}

cmd="$1"
[ $# -gt 0 ] && shift
case $cmd in
    check)
	[ "$1" ] && dev="$1"
	check
	;;
    configure)
	[ "$2" ] && {
	    dev="$1"
	    shift
	}
	configure "$1"
	;;
    *)
	usage
	;;
esac
