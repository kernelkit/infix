################################################################################
#
# image-itb-boot
#
################################################################################

IMAGE_ITB_BOOT_DEPENDENCIES := rootfs-squashfs
IMAGE_ITB_BOOT_CONFIG_VARS := DTB ADDR CONSOLE STRIP_KERNEL

$(eval $(ix-image))
