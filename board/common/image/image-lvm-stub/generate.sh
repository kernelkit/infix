#!/bin/sh

set -e


mkdir -p "${WORKDIR}"/root
rm -rf   "${WORKDIR}"/tmp
mkdir -p "${WORKDIR}"/tmp

"${BR2_EXTERNAL_INFIX_PATH}"/utils/lvm-mkinternal >"${WORKDIR}"/stub.lvm

cat <<EOF >"${WORKDIR}"/genimage.cfg
image lvm-stub.disk {
	hdimage {
		partition-table-type = "gpt"
	}

	partition internal {
		image = "stub.lvm"
		partition-type-uuid = "lvm"
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
