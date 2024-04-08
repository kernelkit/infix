################################################################################
#
# netbrowse
#
################################################################################

NETBROWSE_VERSION = 1.0
NETBROWSE_SITE_METHOD = local
NETBROWSE_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/netbrowse
NETBROWSE_SETUP_TYPE = setuptools
NETBROWSE_LICENSE = MIT
NETBROWSE_LICENSE_FILES = LICENSE
NETBROWSE_REDISTRIBUTE = NO

define NETBROWSE_INSTALL_EXTRA
	$(INSTALL) -D -m 0644 $(NETBROWSE_PKGDIR)/netbrowse.svc \
		$(FINIT_D)/available/netbrowse.conf
	$(INSTALL) -d -m 755 $(TARGET_DIR)/etc/default
	echo "NETBROWSE_ARGS=\"network.local $(BR2_TARGET_GENERIC_HOSTNAME).local\"" \
		> $(TARGET_DIR)/etc/default/netbrowse
endef
NETBROWSE_POST_INSTALL_TARGET_HOOKS += NETBROWSE_INSTALL_EXTRA

$(eval $(python-package))
