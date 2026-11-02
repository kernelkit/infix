################################################################################
#
# image-ddi-disk
#
################################################################################

IMAGE_DDI_DISK_DEPENDENCIES := image-barebox-esp image-ddi
IMAGE_DDI_DISK_CONFIG_VARS := QCOW_SIZE

$(eval $(ix-image))
