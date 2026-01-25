Board Support
=============

The board support for an architecture always starts with Qemu support,
this is what each `linux_defconfig` at the very least sets up.  Then
each `$BR2_ARCH` has additional BSPs, e.g., Banana Pi BPI-R3.

The `board/` directory is matched with the `configs/*_defconfigs` and
the only execption is `board/common/`, which holds all shared files for
Infix builds.

Each `board/$BR2_ARCH/` can then have vendor/product sub-directories
for the BSPs which may contain "fixups" to the base kernel config and
any additional device tree files that should be included as well.

To rebuild a board-specific package, e.g. NanoPi R2S:

    make friendlyarm-nanopi-r2s-rebuild all
