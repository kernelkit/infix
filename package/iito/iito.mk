################################################################################
#
# iito
#
################################################################################

IITO_VERSION = 8c671ea6408ba93318c840460ed7dd25af12eadc
IITO_SITE = $(call github,kernelkit,iito,$(IITO_VERSION))
IITO_LICENSE = GPL-2.0
IITO_LICENSE_FILES = COPYING
IITO_DEPENDENCIES = jansson libev udev
# No configure script in the git tree
IITO_AUTORECONF = YES

define IITO_INSTALL_HOOK
	$(INSTALL) -D -m 0644 $(IITO_PKGDIR)/iitod.svc $(FINIT_D)/available/iitod.conf
	$(INSTALL) -d -m 0755 $(FINIT_D)/enabled
	ln -sf ../available/iitod.conf $(FINIT_D)/enabled/iitod.conf
	cp $(IITO_PKGDIR)/tmpfiles.conf $(TARGET_DIR)/etc/tmpfiles.d/iitod.conf
endef

IITO_POST_INSTALL_TARGET_HOOKS += IITO_INSTALL_HOOK

$(eval $(autotools-package))
