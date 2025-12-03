################################################################################
#
# image-itb-qcow
#
################################################################################

# We can source the rootfs+aux from a local build, or from a
# downloaded release; so adjust our dependencies accordingly.
IMAGE_ITB_QCOW_SRC-$(IMAGE_ITB_ROOTFS)     := image-itb-rootfs image-itb-aux
IMAGE_ITB_QCOW_SRC-$(IMAGE_ITB_DL_RELEASE) := image-itb-dl-release

IMAGE_ITB_QCOW_DEPENDENCIES := host-genimage $(IMAGE_ITB_QCOW_SRC-y)
IMAGE_ITB_QCOW_CONFIG_VARS := BOOT_DATA BOOT_OFFSET SIZE

$(eval $(ix-image))
