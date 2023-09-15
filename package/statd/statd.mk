################################################################################
#
# statd
#
################################################################################

STATD_VERSION = 1.0
STATD_SITE_METHOD = local
STATD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/statd
STATD_LICENSE = BSD-3-Clause
STATD_LICENSE_FILES = LICENSE
STATD_REDISTRIBUTE = NO
STATD_DEPENDENCIES = sysrepo libev libsrx jansson

define STATD_BUILD_CMDS
	$(TARGET_MAKE_ENV) $(TARGET_CONFIGURE_OPTS) $(MAKE) -C $(@D) \
		LDFLAGS="$(TARGET_LDFLAGS)"
endef

define STATD_INSTALL_TARGET_CMDS
	$(TARGET_MAKE_ENV) $(TARGET_CONFIGURE_OPTS) $(MAKE) -C $(@D) \
		DESTDIR="$(TARGET_DIR)" install
endef

define STATD_INSTALL_EXTRA
	cp $(STATD_PKGDIR)/statd.conf  $(FINIT_D)/available/
	ln -sf ../available/statd.conf $(FINIT_D)/enabled/statd.conf
endef
STATD_TARGET_FINALIZE_HOOKS += STATD_INSTALL_EXTRA

$(eval $(generic-package))
