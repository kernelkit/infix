#!/bin/sh
# Download curios-httpd container images for upgrade testing
# This script is called during Docker image build to pre-populate
# the test container with the necessary OCI archives.

set -e

DEST_DIR="${1:-/srv}"
IMAGE_BASE="ghcr.io/kernelkit/curios-httpd"
VERSIONS="24.05.0 24.11.0"
ARCHS="linux/amd64 linux/arm64"

echo "Downloading curios-httpd images to $DEST_DIR..."
mkdir -p "$DEST_DIR"

for ver in $VERSIONS; do
    for arch in $ARCHS; do
        # Create architecture-specific filename
        arch_suffix=$(echo "$arch" | sed 's|linux/||')
        output="$DEST_DIR/curios-httpd-${ver}-${arch_suffix}.tar"

        echo "Fetching ${IMAGE_BASE}:${ver} for ${arch}..."
        skopeo copy --override-arch "${arch#linux/}" \
            "docker://${IMAGE_BASE}:${ver}" \
            "oci-archive:${output}"

        # Check if already gzipped and compress if needed
        output_gz="${output}.gz"
        if file "$output" | grep -q "gzip compressed"; then
            echo "File ${output} is already gzipped, renaming..."
            mv "$output" "$output_gz"
        else
            echo "Compressing ${output}..."
            gzip "${output}"
        fi
    done
done

echo "Download complete!"
ls -lh "$DEST_DIR"
