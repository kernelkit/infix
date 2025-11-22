#!/bin/sh

set -e

squash="${BINARIES_DIR}"/rootfs.squashfs
itbh="${BINARIES_DIR}"/rootfs.itbh
pkg="${BINARIES_DIR}"/"${ARTIFACT}.pkg"

cp -f "${PKGDIR}"/hooks.sh "${WORKDIR}"/hooks.sh

# RAUC internally uses the file extension to find a suitable install
# handler, hence the name must be .img
cp -f "${squash}" "${WORKDIR}"/rootfs.img
cp -f "${itbh}"   "${WORKDIR}"/rootfs.itbh

cat >"${WORKDIR}"/manifest.raucm <<EOF
[update]
compatible=${COMPATIBLE}
version=${VERSION}

[bundle]
format=verity

[hooks]
filename=hooks.sh

[image.rootfs]
filename=rootfs.img
hooks=post-install
EOF

rauc --cert="${CERT}" --key="${KEY}" \
     bundle "${WORKDIR}" "${pkg}.next"

mv "${pkg}.next" "${pkg}"
