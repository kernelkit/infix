#!/bin/bash
set -e

TARGET=sd

BOARD_DIR=$(dirname "$0")
GENIMAGE_CFG="${BUILD_DIR}/genimage.cfg"
GENIMAGE_TMP="${BUILD_DIR}/genimage.tmp"

sed "s|#VERSION#|${RELEASE}|"  "${BOARD_DIR}/genimage.cfg.in" | \
sed "s|#TARGET#|${TARGET}|" | \
sed "s|#INFIX_ID#|${INFIX_ID}|" > "${GENIMAGE_CFG}"

# Create temporary root path
ROOTPATH_TMP=$(mktemp -d)
trap 'rm -rf \"$ROOTPATH_TMP\"' EXIT

# Clean previous genimage temp directory
rm -rf "${GENIMAGE_TMP}"

# Generate the SD card image
genimage					\
    --rootpath   "${ROOTPATH_TMP}"		\
    --tmppath    "${GENIMAGE_TMP}"		\
    --inputpath  "${BINARIES_DIR}"		\
    --outputpath "${BINARIES_DIR}"		\
    --config     "${GENIMAGE_CFG}"
