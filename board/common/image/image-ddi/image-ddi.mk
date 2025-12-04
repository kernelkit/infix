################################################################################
#
# image-ddi
#
################################################################################

IMAGE_DDI_DEPENDENCIES := host-genimage rootfs-squashfs
IMAGE_DDI_CONFIG_VARS := KEY CERT

$(eval $(ix-image))
