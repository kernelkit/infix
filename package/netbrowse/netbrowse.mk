################################################################################
#
# netbrowse
#
################################################################################

NETBROWSE_VERSION = 2.0
NETBROWSE_SITE_METHOD = local
NETBROWSE_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/netbrowse
NETBROWSE_GOMOD = github.com/kernelkit/infix/src/netbrowse
NETBROWSE_LICENSE = MIT
NETBROWSE_LICENSE_FILES = LICENSE
NETBROWSE_REDISTRIBUTE = NO

define NETBROWSE_INSTALL_EXTRA
	$(INSTALL) -D -m 0644 $(NETBROWSE_PKGDIR)/netbrowse.svc \
		$(FINIT_D)/available/netbrowse.conf
	ln -sf ../available/netbrowse.conf $(FINIT_D)/enabled/netbrowse.conf
endef
NETBROWSE_POST_INSTALL_TARGET_HOOKS += NETBROWSE_INSTALL_EXTRA

$(eval $(golang-package))
