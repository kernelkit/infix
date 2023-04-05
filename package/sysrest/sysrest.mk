################################################################################
#
# sysrest
#
################################################################################

SYSREST_VERSION = 1.0
SYSREST_LICENSE = BSD-3-Clause
SYSREST_SITE_METHOD = local
SYSREST_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/sysrest
SYSREST_DEPENDENCIES = sysrepo libfcgi
SYSREST_AUTORECONF = YES

define SYSREST_INSTALL_EXTRA
	cp $(SYSREST_PKGDIR)/sysrest.conf  $(FINIT_D)/available/
	ln -sf ../available/sysrest.conf $(FINIT_D)/enabled/sysrest.conf
endef
SYSREST_TARGET_FINALIZE_HOOKS += SYSREST_INSTALL_EXTRA

$(eval $(autotools-package))
