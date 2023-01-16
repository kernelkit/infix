#!/bin/sh

cat <<EOF >"$BINARIES_DIR/infix.gns3a"
{
    "name": "infix",
    "category": "router",
    "description": "Infix Network Operating System",
    "vendor_name": "KernelKit",
    "vendor_url": "https://github.com/kernelkit/",
    "product_name": "Infix",
    "registry_version": 6,
    "status": "stable",
    "maintainer": "KernelKit",
    "maintainer_email": "infix@example.com",
    "port_name_format": "eth{0}",
    "linked_clone": true,
    "qemu": {
        "adapter_type": "virtio-net-pci",
        "adapters": 10,
        "ram": 192,
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
            "version": "v0.0"
        },
        {
            "filename": "rw.ext4",
            "filesize": $(stat --printf='%s' "$BINARIES_DIR/rw.ext4"),
            "md5sum": "$(md5sum "$BINARIES_DIR/rw.ext4" | awk '{print $1}')",
            "version": "v0.0"
        }
    ],
    "versions": [
        {
            "name": "v0.0",
            "images": {
                "cdrom_image": "rootfs.iso9660",
                "hda_disk_image": "rw.ext4"
            }
        }
    ]
}
EOF
