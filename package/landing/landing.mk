################################################################################
#
# landing
#
################################################################################

LANDING_VERSION = 1.0
LANDING_SITE_METHOD = local
LANDING_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/landing
LANDING_LICENSE = ISC
LANDING_LICENSE_FILES = LICENSE

define LANDING_INSTALL_TARGET_CMDS
	cp $(@D)/*.html $(TARGET_DIR)/usr/html/
	cp $(@D)/*.png  $(TARGET_DIR)/usr/html/
endef

$(eval $(generic-package))
