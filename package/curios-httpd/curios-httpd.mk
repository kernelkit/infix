################################################################################
#
# curios-httpd
#
################################################################################

CURIOS_HTTPD_VERSION = edge
CURIOS_HTTPD_SOURCE = curios-httpd-oci-$(GO_GOARCH).tar.gz
CURIOS_HTTPD_SITE = https://github.com/kernelkit/curiOS/releases/download/$(CURIOS_HTTPD_VERSION)
CURIOS_HTTPD_LICENSE = GPL
CURIOS_HTTPD_INSTALL_TARGET = YES

define CURIOS_HTTPD_INSTALL_TARGET_CMDS
	mkdir -p $(TARGET_DIR)/lib/oci
	(cd $(@D)/.. && $(TAR) cfz $(TARGET_DIR)/lib/oci/$(notdir $(@D)).tar.gz $(notdir $(@D)))
endef

$(eval $(generic-package))
