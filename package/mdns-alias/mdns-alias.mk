################################################################################
#
# mdns-alias
#
################################################################################

MDNS_ALIAS_VERSION = 1.0
MDNS_ALIAS_SITE_METHOD = local
MDNS_ALIAS_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/mdns-alias
MDNS_ALIAS_SETUP_TYPE = setuptools
MDNS_ALIAS_LICENSE = MIT
MDNS_ALIAS_LICENSE_FILES = LICENSE
MDNS_ALIAS_REDISTRIBUTE = NO

define MDNS_ALIAS_INSTALL_EXTRA
	$(INSTALL) -D -m 0644 $(MDNS_ALIAS_PKGDIR)/mdns-alias.svc \
		$(FINIT_D)/available/mdns-alias.conf
	ln -sf ../available/mdns-alias.conf $(FINIT_D)/enabled/mdns-alias.conf
	$(INSTALL) -d -m 755 $(TARGET_DIR)/etc/default
	echo "MDNS_ALIAS_ARGS=\"network.local $(BR2_TARGET_GENERIC_HOSTNAME).local\"" \
		> $(TARGET_DIR)/etc/default/mdns-alias
endef
MDNS_ALIAS_POST_INSTALL_TARGET_HOOKS += MDNS_ALIAS_INSTALL_EXTRA

$(eval $(python-package))
