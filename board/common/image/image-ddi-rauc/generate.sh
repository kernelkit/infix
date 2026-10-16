#!/bin/sh

set -e

ddi="${BINARIES_DIR}"/"${ARTIFACT}.raw"
pkg="${BINARIES_DIR}"/"${ARTIFACT}.pkg"

cp -f "${PKGDIR}"/hooks.sh "${WORKDIR}"/hooks.sh

# RAUC <= 1.15 uses the file extension to find a suitable install
# handler, which must be .img in order to select the "raw" type.
cp -f "${ddi}" "${WORKDIR}"/
ln -sf "${ARTIFACT}.raw" "${WORKDIR}"/ddi.img

cat >"${WORKDIR}"/manifest.raucm <<EOF
[update]
compatible=${COMPATIBLE}
version=${VERSION}

[bundle]
format=verity

[hooks]
filename=hooks.sh

[image.rootfs]
filename=ddi.img
hooks=pre-install
EOF

rauc --cert="${CERT}" --key="${KEY}" \
     bundle "${WORKDIR}" "${pkg}.next"

mv "${pkg}.next" "${pkg}"
