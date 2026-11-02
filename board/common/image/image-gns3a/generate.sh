#!/bin/sh

set -e

qcow="${BINARIES_DIR}"/"${ARTIFACT}".qcow2
gns3a="${BINARIES_DIR}"/"${ARTIFACT}".gns3a

# Same UEFI firmware as the Infix appliance in the GNS3 registry
bios=OVMF-edk2-stable202305.fd
bios_url="https://sourceforge.net/projects/gns-3/files/Qemu%20Appliances/"

cat <<EOF >"${gns3a}"
{
    "name": "${ARTIFACT} devel",
    "category": "router",
    "description": "${ARTIFACT} development appliance",
    "vendor_name": "Kernelkit",
    "vendor_url": "https://www.kernelkit.org",
    "product_name": "${ARTIFACT} devel",
    "registry_version": 6,
    "status": "experimental",
    "maintainer": "Kernelkit",
    "maintainer_email": "null@kernelkit.org",
    "usage": "Default login, user/pass: admin/admin\n\nType 'cli' (and Enter) followed by 'help' for an overview of commands and relevant configuration files.",
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
            "filename": "${bios}",
            "version": "stable202305",
            "md5sum": "6c4cf1519fec4a4b95525d9ae562963a",
            "filesize": 4194304,
            "download_url": "${bios_url}",
            "direct_download_url": "${bios_url}${bios}.zip/download",
            "compression": "zip"
        },
        {
            "filename": "$(basename "${qcow}")",
            "filesize": $(stat --printf='%s' "${qcow}"),
            "md5sum": "$(md5sum "${qcow}" | awk '{print $1}')",
            "version": "${VERSION}"
        }
    ],
    "versions": [
        {
            "name": "${VERSION}",
            "images": {
                "bios_image": "${bios}",
                "hda_disk_image": "$(basename "${qcow}")"
            }
        }
    ]
}
EOF
