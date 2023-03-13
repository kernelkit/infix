################################################################################
#
# mech
#
################################################################################

MECH_VERSION = 1.0
MECH_LICENSE = Apache-2.0
MECH_SITE_METHOD = local
MECH_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/mech
MECH_DEPENDENCIES = augeas clixon
MECH_AUTORECONF = YES

define MECH_INSTALL_EXTRA
	cp $(MECH_PKGDIR)/clixon.conf   $(FINIT_D)/available/
	ln -sf ../available/clixon.conf $(FINIT_D)/enabled/clixon.conf
	cp $(MECH_PKGDIR)/tmpfiles.conf $(TARGET_DIR)/etc/tmpfiles.d/mech.conf
endef
MECH_TARGET_FINALIZE_HOOKS += MECH_INSTALL_EXTRA

$(eval $(autotools-package))
