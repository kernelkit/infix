#!/bin/sh

# Generate the DDI in two phases. First we create the verity hash tree
# and signature, then we collect them together with the rootfs in the
# ddi. The reason for this is that genimage (at least up to v19) can
# not infer the size of a partition if the image to be placed in it is
# created by the same genimage instance. With the two-phase approach,
# we work around that problem and can create a DDI without any wasted
# space.

set -e

rm -rf   "${WORKDIR}"/tmp
mkdir -p "${WORKDIR}"/tmp "${WORKDIR}"/verity

cat <<EOF >"${WORKDIR}"/genimage-verity.cfg
image rootfs.verity {
	verity {
        	image = "rootfs.squashfs"
	}
}

image rootfs.verity-sig {
	verity-sig {
        	image = "rootfs.verity"
		cert = "${CERT}"
		key = "${KEY}"
	}
}

config {}
EOF

genimage \
    --loglevel 1 \
    --tmppath    "${WORKDIR}"/tmp \
    --rootpath   "${WORKDIR}" \
    --inputpath  "${BINARIES_DIR}" \
    --outputpath "${WORKDIR}"/verity \
    --config     "${WORKDIR}"/genimage-verity.cfg

case "${BR2_ARCH}" in
    "x86_64")
	arch=x86-64
	;;
    *)
	echo "ERROR: missing mapping from ${BR2_ARCH} to genimage arch" >&2
	exit 1
	;;
esac

cat <<EOF >"${WORKDIR}"/genimage-ddi.cfg
image ${ARTIFACT}.raw {
	hdimage {
		partition-table-type = "gpt"
	}

	partition root {
		partition-type-uuid = "root-${arch}"
		image = "rootfs.squashfs"
	}
	partition root-verity {
		partition-type-uuid = "root-${arch}-verity"
		image = "${WORKDIR}/verity/rootfs.verity"
	}
	partition root-verity-sig {
		partition-type-uuid = "root-${arch}-verity-sig"
		image = "${WORKDIR}/verity/rootfs.verity-sig"
	}
}

config {}
EOF

genimage \
    --loglevel 1 \
    --tmppath    "${WORKDIR}"/tmp \
    --rootpath   "${WORKDIR}" \
    --inputpath  "${BINARIES_DIR}" \
    --outputpath "${BINARIES_DIR}" \
    --config     "${WORKDIR}"/genimage-ddi.cfg
