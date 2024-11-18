################################################################################
#
# curios-nftables
#
################################################################################

CURIOS_NFTABLES_VERSION = v24.05.0
CURIOS_NFTABLES_SOURCE = curios-nftables-oci-$(GO_GOARCH)-$(CURIOS_NFTABLES_VERSION).tar.gz
CURIOS_NFTABLES_SITE = https://github.com/kernelkit/curiOS/releases/download/$(CURIOS_NFTABLES_VERSION)
CURIOS_NFTABLES_LICENSE = GPL
CURIOS_NFTABLES_LICENSE_FILES = COPYING
CURIOS_NFTABLES_INSTALL_TARGET = YES

define CURIOS_NFTABLES_INSTALL_TARGET_CMDS
	mkdir -p $(TARGET_DIR)/lib/oci
	cp $(CURIOS_NFTABLES_DL_DIR)/$(CURIOS_NFTABLES_SOURCE) \
		$(TARGET_DIR)/lib/oci/$(CURIOS_NFTABLES_NAME)-$(CURIOS_NFTABLES_VERSION).tar.gz
	ln -sf $(CURIOS_NFTABLES_NAME)-$(CURIOS_NFTABLES_VERSION).tar.gz \
		$(TARGET_DIR)/lib/oci/$(CURIOS_NFTABLES_NAME)-latest.tar.gz
endef

$(eval $(generic-package))
