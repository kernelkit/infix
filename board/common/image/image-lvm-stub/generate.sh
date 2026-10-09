#!/bin/sh

set -e


mkdir -p "${WORKDIR}"/root
rm -rf   "${WORKDIR}"/tmp
mkdir -p "${WORKDIR}"/tmp

"${BR2_EXTERNAL_INFIX_PATH}"/utils/lvm-mkinternal >"${WORKDIR}"/stub.lvm

. $BR2_EXTERNAL_INFIX_PATH/board/common/rootfs/etc/partition-uuid
[ -n "${BAREBOX_STATE_UUID}" ]
[ -n "${ESP_UUID}" ]
[ -n "${ESP_BACKUP_UUID}" ]

cat <<EOF >"${WORKDIR}"/genimage.cfg
image lvm-stub.disk {
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
		image = "stub.lvm"
		partition-type-uuid = "lvm"
		size = 2M
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
