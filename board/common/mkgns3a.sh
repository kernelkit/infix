#!/bin/sh
# shellcheck disable=SC1091
. "$TARGET_DIR/etc/os-release"

if [ -n "$INFIX_RELEASE" ]; then
    rel="-${INFIX_RELEASE}"
fi

ARCH=$1
NM="${2:-custom}${rel}"
DISK=$3
RAM=${4:-512}
IFNUM=${5:-1}

# The aarch64 build currently has no "loader" but instead starts Linux
# directly, so we need to add a basic cmdline.
loader_args()
{
    if [ "$ARCH" = "aarch64" ]; then
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

if [ "$ARCH" = "x86_64" ]; then
    loader=OVMF.fd
    accel=allow
    opts=
else
    loader=Image
    accel=disable
    opts="-M virt -cpu cortex-a72"
fi

echo ">> Disk image MD5: $(md5sum "$BINARIES_DIR/$DISK" | awk '{print $1}')"

cat <<EOF >"$BINARIES_DIR/${NM}.gns3a"
{
    "name": "$NM",
    "category": "router",
    "description": "$INFIX_DESC",
    "vendor_name": "$VENDOR_NAME",
    "vendor_url": "$VENDOR_HOME",
    "product_name": "$NAME",
    "registry_version": 6,
    "status": "stable",
    "maintainer": "$VENDOR_NAME",
    "maintainer_email": "${SUPPORT_URL#mailto:}",
    "usage": "Default login, user/pass: admin/admin\n\nType 'cli' (and Enter) followed by 'help' for an overview of commands and relevant configuration files.",
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
        $(loader_img)
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
            "filename": "$DISK",
            "filesize": $(stat --printf='%s' "$BINARIES_DIR/$DISK"),
            "md5sum": "$(md5sum "$BINARIES_DIR/$DISK" | awk '{print $1}')",
            "version": "0.0"
        }
    ],
    "versions": [
        {
            "name": "0.0",
            "images": {
                $(loader_img)
                "hda_disk_image": "$DISK"
            }
        }
    ]
}
EOF
