################################################################################
#
# confd
#
################################################################################

CONFD_VERSION = 1.0
CONFD_LICENSE = BSD-3-Clause
CONFD_SITE_METHOD = local
CONFD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/confd
CONFD_DEPENDENCIES = augeas jansson libite sysrepo
CONFD_AUTORECONF = YES

define CONFD_INSTALL_EXTRA
	cp $(CONFD_PKGDIR)/sysrepo.conf  $(FINIT_D)/available/
	ln -sf ../available/sysrepo.conf $(FINIT_D)/enabled/sysrepo.conf
	cp $(CONFD_PKGDIR)/tmpfiles.conf $(TARGET_DIR)/etc/tmpfiles.d/confd.conf
	mkdir -p $(TARGET_DIR)/usr/share/factory/cfg
	cp $(CONFD_PKGDIR)/factory.cfg   $(TARGET_DIR)/usr/share/factory/cfg/startup-config.cfg
	mkdir -p $(TARGET_DIR)/lib/infix
	cp $(CONFD_PKGDIR)/prep-db       $(TARGET_DIR)/lib/infix/
	cp $(CONFD_PKGDIR)/clean-etc     $(TARGET_DIR)/lib/infix/
endef
CONFD_TARGET_FINALIZE_HOOKS += CONFD_INSTALL_EXTRA

$(eval $(autotools-package))
