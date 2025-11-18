#!/bin/sh

set -e

bios="${BINARIES_DIR}"/OVMF.fd
qcow="${BINARIES_DIR}"/"${ARTIFACT}".qcow2
gns3a="${BINARIES_DIR}"/"${ARTIFACT}".gns3a

cat <<EOF >"${gns3a}"
{
    "name": "${ARTIFACT} devel",
    "category": "router",
    "description": "${ARTIFACT} development appliance",
    "vendor_name": "Kernelkit",
    "vendor_url": "https://kernelkit.org",
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
        "bios_image": "$(basename ${bios})",
        "kvm": "allow"
    },
    "images": [
        {
            "filename": "$(basename "${bios}")",
            "filesize": $(stat --printf='%s' "${bios}"),
            "md5sum": "$(md5sum "${bios}" | awk '{print $1}')",
            "version": "0.0"
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
                "bios_image": "$(basename ${bios})",
                "hda_disk_image": "$(basename ${qcow})"
            }
        }
    ]
}
EOF
