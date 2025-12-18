#!/bin/sh

set -e


mkdir -p "${WORKDIR}"/root
rm -rf   "${WORKDIR}"/tmp
mkdir -p "${WORKDIR}"/tmp

"${BR2_EXTERNAL_INFIX_PATH}"/utils/lvm-mkinternal \
	primary "${BINARIES_DIR}"/"${ARTIFACT}".raw >"${WORKDIR}"/internal.lvm

. $BR2_EXTERNAL_INFIX_PATH/board/common/rootfs/etc/partition-uuid
[ -n "${BAREBOX_STATE_UUID}" ]
[ -n "${ESP_UUID}" ]
[ -n "${ESP_BACKUP_UUID}" ]

cat <<EOF >"${WORKDIR}"/genimage.cfg

image ${ARTIFACT}.disk {
	hdimage {
		partition-table-type = "gpt"
	}

	partition esp {
		partition-type-uuid = "esp"
		partition-uuid = "$ESP_UUID"
		image = "$BINARIES_DIR/barebox-esp.vfat"
	}

	partition esp-backup {
		partition-type-uuid = "esp"
		partition-uuid = "$ESP_BACKUP_UUID"
		image = "$BINARIES_DIR/barebox-esp.vfat"
	}

	partition barebox-state {
		partition-type-uuid = "barebox-state"
		partition-uuid = "$BAREBOX_STATE_UUID"
		size = 2048
	}

	partition internal {
		growfs = "true"
		partition-type-uuid = "lvm"
		image = "internal.lvm"
	}
}

image ${ARTIFACT}.qcow2 {
	qemu {
		format = "qcow2"
	}

	partition disk {
		image = "${ARTIFACT}.disk"
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
