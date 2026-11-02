################################################################################
#
# image-itb-gns3a
#
################################################################################

IMAGE_ITB_GNS3A_DEPENDENCIES := image-itb-qcow
IMAGE_ITB_GNS3A_CONFIG_VARS  := IFNUM RAM

$(eval $(ix-image))
