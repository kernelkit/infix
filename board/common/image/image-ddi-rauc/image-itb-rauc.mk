################################################################################
#
# image-ddi-rauc
#
################################################################################

IMAGE_DDI_RAUC_DEPENDENCIES := host-rauc image-ddi
IMAGE_DDI_RAUC_CONFIG_VARS := KEY CERT

$(eval $(ix-image))
