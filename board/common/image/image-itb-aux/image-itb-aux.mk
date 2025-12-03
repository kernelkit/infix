################################################################################
#
# image-itb-aux
#
################################################################################

IMAGE_ITB_AUX_DEPENDENCIES := host-uboot-tools host-genimage image-itb-rootfs
IMAGE_ITB_AUX_OPTS := $(if $(BR2_TARGET_GRUB2),grub,uboot)

$(eval $(ix-image))
