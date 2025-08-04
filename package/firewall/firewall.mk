################################################################################
#
# Firewall support
#
################################################################################

FIREWALL_PACKAGE_VERSION = 1.0
FIREWALL_PACKAGE_LICENSE = MIT
FIREWALL_DEPENDENCIES = firewalld
FIREWALL_SERVICES_YANG = $(CONFD_SRCDIR)/yang/confd/infix-firewall-services.yang

# Copy custom service definitions and run finalization script
define FIREWALL_INSTALL_CUSTOM_SERVICES
	mkdir -p $(TARGET_DIR)/usr/lib/firewalld/services
	cp $(FIREWALL_PKGDIR)/services/*.xml $(TARGET_DIR)/usr/lib/firewalld/services/
endef

define FIREWALL_FINALIZE
	$(FIREWALL_PKGDIR)/finalize.sh $(TARGET_DIR) $(FIREWALL_SERVICES_YANG)
endef

FIREWALL_POST_INSTALL_TARGET_HOOKS += FIREWALL_INSTALL_CUSTOM_SERVICES
FIREWALL_TARGET_FINALIZE_HOOKS += FIREWALL_FINALIZE

$(eval $(generic-package))
