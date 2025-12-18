#!/bin/sh

set -e


mkdir -p "${WORKDIR}"/root
rm -rf   "${WORKDIR}"/tmp
mkdir -p "${WORKDIR}"/tmp

"${BR2_EXTERNAL_INFIX_PATH}"/utils/lvm-mkinternal \
	"${BINARIES_DIR}"/"${ARTIFACT}".raw >"${WORKDIR}"/internal.lvm

cat <<EOF >"${WORKDIR}"/genimage.cfg
image esp.vfat {
	temporary = "true"
	size = "16M"
	vfat {
		file EFI/BOOT/BOOTX64.EFI {
			image = $BINARIES_DIR/barebox.efi
		}
	}
}

image ${ARTIFACT}.disk {
	hdimage {
		partition-table-type = "gpt"
# TODO: use to detect unprovisioned disks??		disk-uuid = "3493fd02-167f-11f1-bc4d-732398e32f32"
	}

	partition esp {
		partition-type-uuid = "esp"
		image = "esp.vfat"
	}

	partition esp-backup {
		partition-type-uuid = "esp"
		image = "esp.vfat"
	}

	partition internal {
		partition-type-uuid = "lvm"
		image = "internal.lvm"
	}
}

# Silence genimage warnings
config {}
EOF

genimage \
    --tmppath    "${WORKDIR}"/tmp  \
    --rootpath   "${WORKDIR}"/root \
    --inputpath  "${WORKDIR}"      \
    --outputpath "${BINARIES_DIR}" \
    --config "${WORKDIR}"/genimage.cfg
