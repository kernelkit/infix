################################################################################
#
# curios-httpd
#
################################################################################

CURIOS_HTTPD_VERSION = v25.06.0
CURIOS_HTTPD_SOURCE = curios-httpd-oci-$(GO_GOARCH)-$(CURIOS_HTTPD_VERSION).tar.gz
CURIOS_HTTPD_SITE = https://github.com/kernelkit/curiOS/releases/download/$(CURIOS_HTTPD_VERSION)
CURIOS_HTTPD_LICENSE = GPL
CURIOS_HTTPD_LICENSE_FILES = COPYING
CURIOS_HTTPD_DEPENDENCIES = ixcont

# Flattened at build time so the container runs straight from the rootfs
define CURIOS_HTTPD_INSTALL_TARGET_CMDS
	$(call IXCONT_FLATTEN,$(CURIOS_HTTPD_DL_DIR)/$(CURIOS_HTTPD_SOURCE),$(CURIOS_HTTPD_NAME)-$(CURIOS_HTTPD_VERSION))
	ln -sfn $(CURIOS_HTTPD_NAME)-$(CURIOS_HTTPD_VERSION) \
		$(TARGET_DIR)/lib/oci/$(CURIOS_HTTPD_NAME)-latest
endef

$(eval $(generic-package))
