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

# Converted at build time, the container runs straight from the image
define CURIOS_HTTPD_INSTALL_TARGET_CMDS
	$(call IXCONT_SIF,$(CURIOS_HTTPD_DL_DIR)/$(CURIOS_HTTPD_SOURCE),$(CURIOS_HTTPD_NAME)-$(CURIOS_HTTPD_VERSION))
	ln -sf $(CURIOS_HTTPD_NAME)-$(CURIOS_HTTPD_VERSION).sif \
		$(TARGET_DIR)/lib/oci/$(CURIOS_HTTPD_NAME)-latest.sif
endef

$(eval $(generic-package))
