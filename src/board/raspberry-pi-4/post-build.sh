#!/bin/sh

# Armbian firmware installs to /lib/firmware but driver wants the
# file(s) in /lib/firmware/brcm/
if [ -f "${TARGET_DIR}/lib/firmware/BCM4345C0.hcd" ]; then
    mv "${TARGET_DIR}/lib/firmware/BCM4345C0.hcd" "${TARGET_DIR}/lib/firmware/brcm/"
fi
