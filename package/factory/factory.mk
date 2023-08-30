################################################################################
#
# factory
#
################################################################################

FACTORY_VERSION = 1.0
FACTORY_LICENSE = MIT
FACTORY_LICENSE_FILES = LICENSE
FACTORY_SITE_METHOD = local
FACTORY_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/factory
FACTORY_REDISTRIBUTE = NO

define FACTORY_BUILD_CMDS
	$(TARGET_MAKE_ENV) $(TARGET_CONFIGURE_OPTS) $(MAKE) -C $(@D) \
		LDLIBS="$(TARGET_LDFLAGS)"
endef

define FACTORY_INSTALL_TARGET_CMDS
	$(TARGET_MAKE_ENV) $(TARGET_CONFIGURE_OPTS) $(MAKE) -C $(@D) \
		DESTDIR="$(TARGET_DIR)" install
endef

$(eval $(generic-package))
