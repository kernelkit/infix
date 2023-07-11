################################################################################
#
# confd
#
################################################################################

CONFD_VERSION = 1.0
CONFD_LICENSE = BSD-3-Clause
CONFD_SITE_METHOD = local
CONFD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/confd
CONFD_LICENSE = BSD-3
CONFD_LICENSE_FILES = LICENSE
CONFD_DEPENDENCIES = augeas jansson libite sysrepo
CONFD_AUTORECONF = YES

ifeq ($(BR2_SYSTEM_BIN_SH_BASH),y)
CONFD_CONF_OPTS += --with-login-shell=/bin/bash
else
CONFD_CONF_OPTS += --with-login-shell=/bin/sh
endif

define CONFD_INSTALL_EXTRA
	cp $(CONFD_PKGDIR)/sysrepo.conf  $(FINIT_D)/available/
	ln -sf ../available/sysrepo.conf $(FINIT_D)/enabled/sysrepo.conf
	cp $(CONFD_PKGDIR)/tmpfiles.conf $(TARGET_DIR)/etc/tmpfiles.d/confd.conf
endef
CONFD_POST_INSTALL_TARGET_HOOKS += CONFD_INSTALL_EXTRA

$(eval $(autotools-package))
