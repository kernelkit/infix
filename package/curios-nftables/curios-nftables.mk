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

# Flattened at build time so the container runs straight from the rootfs
define CURIOS_NFTABLES_INSTALL_TARGET_CMDS
	$(call IXCONT_FLATTEN,$(CURIOS_NFTABLES_DL_DIR)/$(CURIOS_NFTABLES_SOURCE),$(CURIOS_NFTABLES_NAME)-$(CURIOS_NFTABLES_VERSION))
	ln -sfn $(CURIOS_NFTABLES_NAME)-$(CURIOS_NFTABLES_VERSION) \
		$(TARGET_DIR)/lib/oci/$(CURIOS_NFTABLES_NAME)-latest
endef

$(eval $(generic-package))
