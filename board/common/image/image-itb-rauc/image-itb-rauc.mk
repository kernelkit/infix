################################################################################
#
# image-itb-rauc
#
################################################################################

IMAGE_ITB_RAUC_DEPENDENCIES := host-rauc image-itb-rootfs
IMAGE_ITB_RAUC_CONFIG_VARS := KEY CERT

$(eval $(ix-image))
