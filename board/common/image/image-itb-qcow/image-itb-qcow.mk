################################################################################
#
# image-itb-qcow
#
################################################################################

IMAGE_ITB_QCOW_DEPENDENCIES := host-genimage image-itb-rootfs image-itb-aux
IMAGE_ITB_QCOW_CONFIG_VARS := BOOT_DATA BOOT_OFFSET SIZE

$(eval $(ix-image))
