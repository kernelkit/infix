################################################################################
#
# image-ext4-rauc
#
################################################################################

IMAGE_EXT4_RAUC_DEPENDENCIES := host-rauc
IMAGE_EXT4_RAUC_CONFIG_VARS := KEY CERT

$(eval $(ix-image))
