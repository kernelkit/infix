#!/bin/sh

# The aarch64 build currently has no "loader" but instead starts Linux
# directly, so we need to add a basic cmdline.
loader_args()
{
    if [ "$ARCH" != "x86_64" ]; then
	cat <<EOF
"kernel_command_line": "console=ttyAMA0 root=PARTLABEL=primary quiet",
EOF
    fi
}

loader_img()
{
    if [ "$ARCH" = "x86_64" ]; then
	cat <<EOF
"bios_image": "$loader",
EOF
    else
	cat <<EOF
"kernel_image": "$loader",
EOF
    fi
}

if [ -n "$INFIX_RELEASE" ]; then
    rel="-${INFIX_RELEASE}"
fi

ARCH=$1
NM="${2:-custom}${rel}"
RAM=${3:-512}
IFNUM=${4:-1}

if [ "$ARCH" = "x86_64" ]; then
    loader=OVMF.fd
    accel=allow
    opts=
else
    loader=Image
    accel=disable
    opts="-M virt -cpu cortex-a72"
fi

cat <<EOF >"$BINARIES_DIR/${NM}.gns3a"
{
    "name": "$NM",
    "category": "router",
    "description": "Infix is a Network Operating System based on Linux.  It can be set up both as a switch, with offloading using switchdev, and a router with firewalling.",
    "vendor_name": "KernelKit",
    "vendor_url": "https://github.com/kernelkit/",
    "product_name": "Infix",
    "registry_version": 6,
    "status": "stable",
    "maintainer": "KernelKit",
    "maintainer_email": "kernelkit@googlegroups.com",
    "usage": "Default console login is 'root', no password.  For remote login, default user/pass: admin/admin also works.\n\nType 'help' for an overview of commands and relevant configuration files.\n\nThe /etc directory is writable, use the passwd tool after login as part of your set up.\nFor networking, classify interfaces as switchports with /etc/mactab, syntax: 'MAC-address  eN', where N is the port number (1-MAX).\nTo set up bridging and management interfaces, use /etc/network/interfaces, and /etc/network/interfaces.d/",
    "port_name_format": "eth{0}",
    "linked_clone": true,
    "qemu": {
        "adapter_type": "virtio-net-pci",
        "adapters": ${IFNUM},
        "ram": ${RAM},
        "cpus": 1,
        "hda_disk_interface": "virtio",
        "arch": "$ARCH",
        "console_type": "telnet",
        $(loader_args)
        "kvm": "$accel",
	"options": "$opts"
    },
    "images": [
        {
            "filename": "$loader",
            "filesize": $(stat --printf='%s' "$BINARIES_DIR/$loader"),
            "md5sum": "$(md5sum "$BINARIES_DIR/$loader" | awk '{print $1}')",
            "version": "0.0"
        },
        {
            "filename": "disk.img",
            "filesize": $(stat --printf='%s' "$BINARIES_DIR/disk.img"),
            "md5sum": "$(md5sum "$BINARIES_DIR/disk.img" | awk '{print $1}')",
            "version": "0.0"
        }
    ],
    "versions": [
        {
            "name": "0.0",
            "images": {
                $(loader_img)
                "hda_disk_image": "disk.img"
            }
        }
    ]
}
EOF
