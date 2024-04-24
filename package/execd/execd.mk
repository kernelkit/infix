################################################################################
#
# execd
#
################################################################################

EXECD_VERSION = 1.0
EXECD_SITE_METHOD = local
EXECD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/execd
EXECD_LICENSE = ISC
EXECD_LICENSE_FILES = LICENSE
EXECD_REDISTRIBUTE = NO
EXECD_DEPENDENCIES = libuev libite
EXECD_AUTORECONF = YES

define EXECD_CONF_ENV
CFLAGS="$(INFIX_CFLAGS)"
endef

EXECD_CONF_OPTS = --prefix= --disable-silent-rules

define EXECD_INSTALL_EXTRA
	cp $(EXECD_PKGDIR)/execd.conf  $(FINIT_D)/available/
	ln -sf ../available/execd.conf $(FINIT_D)/enabled/execd.conf
	cp $(EXECD_PKGDIR)/tmpfiles.conf $(TARGET_DIR)/lib/tmpfiles.d/execd.conf
endef
EXECD_TARGET_FINALIZE_HOOKS += EXECD_INSTALL_EXTRA

$(eval $(autotools-package))
