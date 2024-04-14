################################################################################
#
# mdns-alias
#
################################################################################

MDNS_ALIAS_VERSION = 1.0
MDNS_ALIAS_SITE = https://github.com/troglobit/mdns-alias/releases/download/v$(MDNS_ALIAS_VERSION)
MDNS_ALIAS_LICENSE = ISC
MDNS_ALIAS_LICENSE_FILES = LICENSE
MDNS_ALIAS_DEPENDENCIES = host-pkgconf avahi
#MDNS_ALIAS_AUTORECONF = YES
#MDNS_ALIAS_DEPENDENCIES += host-automake host-autoconf

define MDNS_ALIAS_INSTALL_EXTRA
	$(INSTALL) -D -m 0644 $(MDNS_ALIAS_PKGDIR)/mdns-alias.svc \
		$(FINIT_D)/available/mdns-alias.conf
	ln -sf ../available/mdns-alias.conf $(FINIT_D)/enabled/mdns-alias.conf
	$(INSTALL) -d -m 755 $(TARGET_DIR)/etc/default
	echo "MDNS_ALIAS_ARGS=\"network.local $(BR2_TARGET_GENERIC_HOSTNAME).local\"" \
		> $(TARGET_DIR)/etc/default/mdns-alias
endef
MDNS_ALIAS_POST_INSTALL_TARGET_HOOKS += MDNS_ALIAS_INSTALL_EXTRA

$(eval $(autotools-package))
