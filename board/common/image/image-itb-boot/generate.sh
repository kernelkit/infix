#!/bin/sh
# Pack the kernel, every device tree, and the root filesystem into one FIT,
# so that a bootloader without BLKMAP, SquashFS, and sysboot can still boot
# Infix, with a single bootm and a single file to fetch.
#
# One configuration per device tree, named after it, so the same image
# boots any board in the build with bootm <addr>#<board>.  The 'boot'
# configuration is the board this build was configured for, and is what
# the boot script uses.
#
# Nothing in the image is tied to a memory map.  The kernel is a
# compressed kernel_noload, so U-Boot finds room to unpack it and then
# moves it to wherever the board's DRAM begins; the ramdisk and device
# trees stay where they were fetched.  The one address there is lives in
# the boot script, and says where to fetch to.
#
# The image is not signed.  Nothing that needs it can check a signature,
# the verified path is rootfs.itb by way of sysboot, and mkimage 2024.04
# reports an error signing a FIT of this shape that we have not run to
# ground.

set -e

kernel="${BINARIES_DIR}"/Image
squash="${BINARIES_DIR}"/rootfs.squashfs
itb="${BINARIES_DIR}"/boot.itb
scr="${BINARIES_DIR}"/boot.scr
work="${WORKDIR}"

dtb="${TARGET_DIR}"/boot/"${DTB}"
if [ ! -f "${dtb}" ]; then
    echo "NO SUCH DEVICE TREE: ${DTB}" >&2
    exit 1
fi
default=$(basename "${DTB}" .dtb)

# A third of the transfer is kernel, and it compresses three to one.
gzip -9 -n <"${kernel}" >"${work}"/Image.gz

# Leave out the copy of the kernel the rootfs carries in /boot, it is
# already in the FIT.  Under fakeroot so ownership survives the round
# trip, and with the same options the rootfs was built with.
if [ "${STRIP_KERNEL}" = "y" ]; then
    bs=$(unsquashfs -s "${squash}" | sed -n 's/^Block size //p')
    comp=$(unsquashfs -s "${squash}" | sed -n 's/^Compression //p')
    rm -rf "${work}"/rootfs
    fakeroot -- sh -c "
	unsquashfs -n -d '${work}/rootfs' '${squash}' &&
	rm -f '${work}'/rootfs/boot/*Image &&
	mksquashfs '${work}/rootfs' '${work}/rootfs.squashfs' \
		-noappend -no-progress -processors $(nproc) \
		-b ${bs} -comp ${comp}"
    squash="${work}"/rootfs.squashfs
fi

# One fdt image and one configuration per device tree.  The device trees
# go last in the image on purpose: U-Boot grows the selected tree in
# place to add the kernel command line and the ramdisk bounds, and after
# the ramdisk that growth only ever reaches another board's tree or the
# end of the image.
: >"${work}"/fdts.itsi
: >"${work}"/cfgs.itsi
for f in $(find "${TARGET_DIR}"/boot -name '*.dtb' | sort); do
    name=$(basename "${f}" .dtb)

    cat >>"${work}"/fdts.itsi <<EOF
		fdt-${name} {
			description = "${f#${TARGET_DIR}/boot/}";
			type = "flat_dt";
			arch = "arm64";
			compression = "none";
			data = /incbin/("${f}");
		};

EOF
    cat >>"${work}"/cfgs.itsi <<EOF
		${name} {
			description = "${name}";
			kernel = "kernel";
			fdt = "fdt-${name}";
			ramdisk = "rootfs";
		};

EOF
done

cat >"${work}"/boot.its <<EOF
/dts-v1/;

/ {
	description = "${ARTIFACT} ${VERSION}";
	creator = "infix";
	#address-cells = <0x1>;

	images {
		kernel {
			description = "Linux";
			type = "kernel_noload";
			arch = "arm64";
			os = "linux";
			compression = "gzip";
			/*
			 * Ignored for a compressed kernel_noload, U-Boot
			 * allocates the decompression buffer and relocates the
			 * arm64 Image itself, but bootm_find_os() refuses a
			 * kernel node without them.
			 */
			load = <0>;
			entry = <0>;
			data = /incbin/("${work}/Image.gz");
		};

		rootfs {
			description = "rootfs";
			type = "ramdisk";
			arch = "arm64";
			os = "linux";
			compression = "none";
			data = /incbin/("${squash}");
		};

$(cat "${work}"/fdts.itsi)
	};

	configurations {
		default = "boot";

		boot {
			description = "${ARTIFACT}, ${default}";
			kernel = "kernel";
			fdt = "fdt-${default}";
			ramdisk = "rootfs";
		};

$(cat "${work}"/cfgs.itsi)
	};
};
EOF

# External data, each blob on a 4 KiB boundary.  No fixed offset for it,
# unlike rootfs.itb: with a configuration per board the header outgrows
# 4 KiB, and mkimage places the data after it either way.
mkimage -E -B 0x1000 -f "${work}"/boot.its "${itb}"

# Boot script for handing out as the DHCP boot file, so a test system
# needs no typing at the prompt.  The image sits next to this script on
# the server, so its name comes from ${bootfile} rather than being built
# in, and the script works wherever the two are served from.
#
# A board that cannot fetch its image resets and tries again rather than
# falling through to whatever bootcmd has left, which on a rack of test
# units is the vendor OS on eMMC.  The pause keeps a server that is down
# from being hammered by the whole rack at once.
cat >"${work}"/boot.cmd <<EOF
setexpr img sub "[.]scr\$" ".itb" "\${bootfile}"

setenv fdt_high 0xffffffffffffffff
setenv initrd_high 0xffffffffffffffff

if tftp ${ADDR} "\${img}"; then
	fdt addr ${ADDR}
	fdt get value rdsz /images/rootfs data-size
	setexpr rdkb \${rdsz} / 0x400
	setenv bootargs "console=${CONSOLE} root=/dev/ram0 ro brd.rd_size=0x\${rdkb} rauc.slot=net loglevel=4 usbcore.authorized_default=2"
	bootm ${ADDR}#boot
fi

echo "Netboot failed, no \${img} on \${serverip}, retrying"
sleep 5
reset
EOF

mkimage -T script -C none -n "${ARTIFACT} netboot" \
	-d "${work}"/boot.cmd "${scr}"
