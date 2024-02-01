################################################################################
#
# keyack
#
################################################################################

KEYACK_VERSION = 1.0
KEYACK_LICENSE = MIT
KEYACK_LICENSE_FILES = LICENSE
KEYACK_SITE_METHOD = local
KEYACK_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/keyack
KEYACK_REDISTRIBUTE = NO

define KEYACK_BUILD_CMDS
	$(TARGET_MAKE_ENV) $(TARGET_CONFIGURE_OPTS) $(MAKE) -C $(@D) \
		LDLIBS="$(TARGET_LDFLAGS)"
endef

define KEYACK_INSTALL_TARGET_CMDS
	$(TARGET_MAKE_ENV) $(TARGET_CONFIGURE_OPTS) $(MAKE) -C $(@D) \
		DESTDIR="$(TARGET_DIR)" install
endef

$(eval $(generic-package))
