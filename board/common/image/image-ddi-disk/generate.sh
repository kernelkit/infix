#!/bin/sh

set -e


mkdir -p "${WORKDIR}"/root
rm -rf   "${WORKDIR}"/tmp
mkdir -p "${WORKDIR}"/tmp

"${BR2_EXTERNAL_INFIX_PATH}"/utils/lvm-mkinternal \
	"${BINARIES_DIR}"/"${ARTIFACT}".raw >"${WORKDIR}"/internal.lvm

cat <<EOF >"${WORKDIR}"/genimage.cfg

image ${ARTIFACT}.disk {
	hdimage {
		partition-table-type = "gpt"
	}

	partition esp {
		partition-type-uuid = "esp"
		image = "$BINARIES_DIR/barebox-esp.vfat"
	}

	partition esp-backup {
		partition-type-uuid = "esp"
		image = "$BINARIES_DIR/barebox-esp.vfat"
	}

	partition internal {
		growfs = "true"
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
