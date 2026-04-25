#!/bin/sh

set -e

ext2="${BINARIES_DIR}/rootfs.ext2"
pkg="${BINARIES_DIR}/${ARTIFACT}-ext4.pkg"

# RAUC internally uses the file extension to find a suitable install
# handler, hence the name must be .img
cp -f "${ext2}" "${WORKDIR}/rootfs.img"

cat >"${WORKDIR}/manifest.raucm" <<EOF
[update]
compatible=${COMPATIBLE}
version=${VERSION}

[bundle]
format=verity

[image.rootfs]
filename=rootfs.img
EOF

rauc --cert="${CERT}" --key="${KEY}" \
     bundle "${WORKDIR}" "${pkg}.next"

mv "${pkg}.next" "${pkg}"
