################################################################################
#
# show
#
################################################################################

SHOW_VERSION = 1.0
SHOW_LICENSE = MIT
SHOW_LICENSE_FILES = LICENSE
SHOW_SITE_METHOD = local
SHOW_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/show
SHOW_REDISTRIBUTE = NO

define SHOW_INSTALL_TARGET_CMDS
	$(TARGET_MAKE_ENV) $(TARGET_CONFIGURE_OPTS) $(MAKE) -C $(@D) \
		DESTDIR="$(TARGET_DIR)" install
endef

$(eval $(generic-package))
