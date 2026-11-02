################################################################################
#
# image-gns3a
#
################################################################################

IMAGE_GNS3A_QCOW-$(IX_IMAGE_DDI_DISK) := image-ddi-disk
IMAGE_GNS3A_QCOW-$(IX_IMAGE_ITB_QCOW) := image-itb-qcow

IMAGE_GNS3A_DEPENDENCIES := $(IMAGE_GNS3A_QCOW-y)
IMAGE_GNS3A_CONFIG_VARS  := IFNUM RAM

$(eval $(ix-image))
