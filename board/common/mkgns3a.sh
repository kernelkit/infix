#!/bin/sh

cat <<EOF >"$BINARIES_DIR/infix.gns3a"
{
    "name": "infix",
    "category": "router",
    "description": "Infix is a Network Operating System based on Linux.  It can be set up both as a switch, with offloading using switchdev, and a router with firewalling.",
    "vendor_name": "KernelKit",
    "vendor_url": "https://github.com/kernelkit/",
    "product_name": "Infix",
    "registry_version": 6,
    "status": "stable",
    "maintainer": "KernelKit",
    "maintainer_email": "kernelkit@googlegroups.com",
    "usage": "Default login is 'root', no password.\n\nType 'help' for an overview of commands and relevant configuration files.\n\nThe /etc directory is writable, use the passwd tool after login as part of your set up.\nFor networking, classify interfaces as switchports with /etc/mactab, syntax: 'MAC-address  eN', where N is the port number (1-MAX).\nTo set up bridging and management interfaces, use /etc/network/interfaces, and /etc/network/interfaces.d/",
    "port_name_format": "eth{0}",
    "linked_clone": true,
    "qemu": {
        "adapter_type": "virtio-net-pci",
        "adapters": 10,
        "ram": 512,
        "cpus": 1,
        "hda_disk_interface": "virtio",
        "arch": "x86_64",
        "console_type": "telnet",
        "kvm": "allow"
    },
    "images": [
        {
            "filename": "rootfs.iso9660",
            "filesize": $(stat --printf='%s' "$BINARIES_DIR/rootfs.iso9660"),
            "md5sum": "$(md5sum "$BINARIES_DIR/rootfs.iso9660" | awk '{print $1}')",
            "version": "0.0"
        },
        {
            "filename": "cfg.ext4",
            "filesize": $(stat --printf='%s' "$BINARIES_DIR/cfg.ext4"),
            "md5sum": "$(md5sum "$BINARIES_DIR/cfg.ext4" | awk '{print $1}')",
            "version": "0.0"
        }
    ],
    "versions": [
        {
            "name": "0.0",
            "images": {
                "cdrom_image": "rootfs.iso9660",
                "hda_disk_image": "cfg.ext4"
            }
        }
    ]
}
EOF
