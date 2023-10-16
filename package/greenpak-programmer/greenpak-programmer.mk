################################################################################
#
# greenpak-programmer
#
################################################################################

GREENPAK_PROGRAMMER_VERSION = 1.0
GREENPAK_PROGRAMMER_SITE = https://github.com/addiva-elektronik/greenpak-programmer/releases/download/v$(GREENPAK_PROGRAMMER_VERSION)
GREENPAK_PROGRAMMER_LICENSE = MIT
GREENPAK_PROGRAMMER_INSTALL_STAGING = YES
GREENPAK_PROGRAMMER_DEPENDENCIES = i2c-tools

define GREENPAK_PROGRAMMER_BUILD_CMDS
	$(TARGET_CONFIGURE_OPTS) $(MAKE) -C $(@D) all
endef

define GREENPAK_PROGRAMMER_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0755 $(@D)/greenpak-programmer $(TARGET_DIR)/usr/bin/greenpak-programmer
endef

$(eval $(generic-package))
