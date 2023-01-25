################################################################################
#
# tetris
#
################################################################################

TETRIS_VERSION = 1.4.0
TETRIS_SITE = https://github.com/troglobit/tetris/releases/download/$(TETRIS_VERSION)
TETRIS_LICENSE = ISC
TETRIS_LICENSE_FILES = LICENSE
TETRIS_INSTALL_STAGING = YES

define TETRIS_BUILD_CMDS
	$(TARGET_CONFIGURE_OPTS) $(MAKE) -C $(@D) all
endef

define TETRIS_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0755 $(@D)/tetris $(TARGET_DIR)/usr/bin/tetris
endef

$(eval $(generic-package))
