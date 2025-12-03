#!/bin/sh

set -e

squash="${BINARIES_DIR}"/rootfs.squashfs
aux="${BINARIES_DIR}"/aux.ext4

[ -f "${squash}" ] && [ -f "${aux}" ] && exit 0

archive="${WORKDIR}/$(basename ${URL})"
[ -f "${archive}" ] || wget -O"${archive}" "${URL}"

echo "Unpacking..."
tar -xa --strip-components=1 -C "${BINARIES_DIR}" -f "${archive}"

auxsize=$(stat -c %s "${aux}")
if [ "${auxsize}" -gt $((8 << 20)) ]; then
    # In older releases, 16M aux.ext4 images were generated. In order
    # to keep the image-itb-qcow logic simpler, trim it 8M, which we
    # always generate nowadays.
    echo "WARNING: Auxiliary partition is unexpectedly large. Resizing..."
    resize2fs "${aux}" 8M
    truncate -s 8M "${aux}"
    tune2fs -l "${aux}"
fi
