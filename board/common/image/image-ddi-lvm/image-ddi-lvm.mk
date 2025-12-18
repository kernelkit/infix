################################################################################
#
# image-ddi-lvm
#
################################################################################

IMAGE_DDI_LVM_DEPENDENCIES := image-ddi
#IMAGE_DDI_LVM_CONFIG_VARS := KEY CERT

$(eval $(ix-image))
