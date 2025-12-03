################################################################################
#
# image-itb-rootfs
#
################################################################################

IMAGE_ITB_ROOTFS_DEPENDENCIES := rootfs-squashfs
IMAGE_ITB_ROOTFS_CONFIG_VARS := KEY

$(eval $(ix-image))
