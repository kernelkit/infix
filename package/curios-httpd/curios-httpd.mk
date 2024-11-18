################################################################################
#
# curios-httpd
#
################################################################################

CURIOS_HTTPD_VERSION = v24.05.0
CURIOS_HTTPD_SOURCE = curios-httpd-oci-$(GO_GOARCH)-$(CURIOS_HTTPD_VERSION).tar.gz
CURIOS_HTTPD_SITE = https://github.com/kernelkit/curiOS/releases/download/$(CURIOS_HTTPD_VERSION)
CURIOS_HTTPD_LICENSE = GPL
CURIOS_HTTPD_LICENSE_FILES = COPYING

define CURIOS_HTTPD_INSTALL_TARGET_CMDS
	mkdir -p $(TARGET_DIR)/lib/oci
	cp $(CURIOS_HTTPD_DL_DIR)/$(CURIOS_HTTPD_SOURCE) \
		$(TARGET_DIR)/lib/oci/$(CURIOS_HTTPD_NAME)-$(CURIOS_HTTPD_VERSION).tar.gz
	ln -sf $(CURIOS_HTTPD_NAME)-$(CURIOS_HTTPD_VERSION).tar.gz \
		$(TARGET_DIR)/lib/oci/$(CURIOS_HTTPD_NAME)-latest.tar.gz
endef

$(eval $(generic-package))
