################################################################################
#
# curios-nftables
#
################################################################################

CURIOS_NFTABLES_VERSION = v25.06.0
CURIOS_NFTABLES_SOURCE = curios-nftables-oci-$(GO_GOARCH)-$(CURIOS_NFTABLES_VERSION).tar.gz
CURIOS_NFTABLES_SITE = https://github.com/kernelkit/curiOS/releases/download/$(CURIOS_NFTABLES_VERSION)
CURIOS_NFTABLES_LICENSE = GPL
CURIOS_NFTABLES_LICENSE_FILES = COPYING
CURIOS_NFTABLES_INSTALL_TARGET = YES
CURIOS_NFTABLES_DEPENDENCIES = ixcont

# Converted at build time, the container runs straight from the image
define CURIOS_NFTABLES_INSTALL_TARGET_CMDS
	$(call IXCONT_SIF,$(CURIOS_NFTABLES_DL_DIR)/$(CURIOS_NFTABLES_SOURCE),$(CURIOS_NFTABLES_NAME)-$(CURIOS_NFTABLES_VERSION))
	ln -sf $(CURIOS_NFTABLES_NAME)-$(CURIOS_NFTABLES_VERSION).sif \
		$(TARGET_DIR)/lib/oci/$(CURIOS_NFTABLES_NAME)-latest.sif
endef

$(eval $(generic-package))
