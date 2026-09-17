################################################################################
#
# support
#
################################################################################

SUPPORT_VERSION = 1.0
SUPPORT_SITE_METHOD = local
SUPPORT_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/support
SUPPORT_LICENSE = ISC
SUPPORT_LICENSE_FILES = LICENSE
SUPPORT_REDISTRIBUTE = NO

define SUPPORT_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0755 $(@D)/support $(TARGET_DIR)/usr/sbin/support
endef

$(eval $(generic-package))
