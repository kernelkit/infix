#!/bin/sh

NM="infix-${1:-custom}"
RAM=${2:-512}
IFNUM=${3:-1}

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
        "arch": "x86_64",
        "console_type": "telnet",
        "kvm": "allow"
    },
    "images": [
        {
            "filename": "OVMF.fd",
            "filesize": $(stat --printf='%s' "$BINARIES_DIR/OVMF.fd"),
            "md5sum": "$(md5sum "$BINARIES_DIR/OVMF.fd" | awk '{print $1}')",
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
                "bios_image": "OVMF.fd",
                "hda_disk_image": "disk.img"
            }
        }
    ]
}
EOF
