#!/bin/sh

set -e

# Bootstrap a RAUC status file showing the newly created image
# installed to both the primary and secondary slots. This then bundled
# in the aux partition in mkdisk.sh, so that RAUC (on the target) can
# always report the installed versions.
rauc info --no-verify --output-format=shell $1 >/tmp/rauc-$$.info
. /tmp/rauc-$$.info
rm /tmp/rauc-$$.info
tstamp=$(date -u +%FT%TZ)
cat <<EOF
[slot.rootfs.0]
bundle.compatible=$RAUC_MF_COMPATIBLE
bundle.version=$RAUC_MF_VERSION
status=ok
sha256=$RAUC_IMAGE_DIGEST_0
size=$RAUC_IMAGE_SIZE_0
installed.timestamp=$tstamp
installed.count=1
activated.timestamp=$tstamp
activated.count=1

[slot.rootfs.1]
bundle.compatible=$RAUC_MF_COMPATIBLE
bundle.version=$RAUC_MF_VERSION
status=ok
sha256=$RAUC_IMAGE_DIGEST_0
size=$RAUC_IMAGE_SIZE_0
installed.timestamp=$tstamp
installed.count=1
activated.timestamp=$tstamp
activated.count=1
EOF
