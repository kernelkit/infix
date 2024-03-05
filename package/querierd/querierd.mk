################################################################################
#
# querierd
#
################################################################################

QUERIERD_VERSION = 0.10
QUERIERD_SITE    = \
	https://github.com/westermo/querierd/releases/download/v$(QUERIERD_VERSION)
QUERIERD_LICENSE = BSD-3-Clause
QUERIERD_LICENSE_FILES = LICENSE
QUERIERD_INSTALL_STAGING = YES

define QUERIERD_INSTALL_CONFIG
	$(INSTALL) -D -m 0644 $(QUERIERD_PKGDIR)/querierd.conf \
		$(TARGET_DIR)/etc/querierd/example.conf
endef
QUERIERD_POST_INSTALL_TARGET_HOOKS += QUERIERD_INSTALL_CONFIG

define QUERIERD_INSTALL_FINIT_SVC
	$(INSTALL) -D -m 0644 $(QUERIERD_PKGDIR)/querierd.svc \
		$(FINIT_D)/available/querierd.conf
	$(INSTALL) -D -m 0644 $(QUERIERD_PKGDIR)/template.svc \
		$(FINIT_D)/available/querierd@.conf
endef

QUERIERD_POST_INSTALL_TARGET_HOOKS += QUERIERD_INSTALL_FINIT_SVC

$(eval $(autotools-package))
