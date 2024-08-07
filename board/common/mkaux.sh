#!/bin/sh

rootdir=$BUILD_DIR/genimage.root
tempdir=$BUILD_DIR/genimage.tmp

cat <<EOF > /tmp/mkaux.cfg
image aux.ext4 {
	mountpoint = "/aux"
	size = 16M

	ext4 {
		label = "aux"
		use-mke2fs = true
	}
}

# Silence genimage warnings
config {}
EOF

rm -rf   "$rootdir/aux"
mkdir -p "$rootdir/aux"
cp -f "$BINARIES_DIR/rootfs.itbh" "$rootdir/aux/primary.itbh"
cp -f "$BINARIES_DIR/rootfs.itbh" "$rootdir/aux/secondary.itbh"
cp -f "$BINARIES_DIR/rauc.status" "$rootdir/aux/rauc.status"

mkenvimage -s 0x4000 -o "$rootdir/aux/uboot.env" \
	   "$BR2_EXTERNAL_INFIX_PATH/board/common/uboot/aux-env.txt"

rm -rf "$BINARIES_DIR/aux.ext4"
rm -rf "$tempdir"

genimage \
    --rootpath   "$rootdir" \
    --tmppath    "$tempdir" \
    --inputpath  "$BINARIES_DIR" \
    --outputpath "$BINARIES_DIR" \
    --config "/tmp/mkaux.cfg"
