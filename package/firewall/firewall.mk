################################################################################
#
# Firewall support
#
################################################################################

FIREWALL_PACKAGE_VERSION = 1.0
FIREWALL_PACKAGE_LICENSE = MIT
FIREWALL_DEPENDENCIES = firewalld
FIREWALL_SERVICES_YANG = $(CONFD_SRCDIR)/yang/confd/infix-firewall-services.yang
FIREWALL_DAEMON_DIR = $(TARGET_DIR)/usr/lib/firewalld
FIREWALL_XML_FILES = $(wildcard $(FIREWALL_DAEMON_DIR)/policies/*.xml \
				$(FIREWALL_DAEMON_DIR)/zones/*.xml)

# The three zones that are *not* deleted from the default install
# are required by firewalld (core/fw.py), in particular block.xml
# Firewalld services cleanup: keep only services that match YANG enums,
# remove all others, and validate that all enums have corresponding .xml files
define FIREWALL_CLEANUP
	rm -rf $(TARGET_DIR)/etc/firewall*
	rm -f  $(TARGET_DIR)/usr/bin/firewall-applet
	rm -rf $(TARGET_DIR)/usr/share/firewalld
	find   $(FIREWALL_DAEMON_DIR)/zones -type f \
		! -name block.xml   \
		! -name drop.xml    \
		! -name trusted.xml \
		-delete
	mkdir -p $(TARGET_DIR)/etc/firewalld/zones
	mkdir -p $(TARGET_DIR)/etc/firewalld/policies
	mkdir -p $(TARGET_DIR)/etc/firewalld/services
	touch    $(TARGET_DIR)/etc/firewalld/firewalld.conf
	mkdir -p $(FIREWALL_DAEMON_DIR)/services
	cp $(FIREWALL_PKGDIR)/services/*.xml $(FIREWALL_DAEMON_DIR)/services/
endef

# Find all built-in services in our YANG services model,
# drop firewalld service.xml that are *not* enumerated.
define FIREWALL_PRUNE_SERVICES
	if [ ! -f "$(FIREWALL_SERVICES_YANG)" ]; then					\
		echo "ERROR: $(FIREWALL_SERVICES_YANG) not found";			\
		exit 1;									\
	fi;										\
	ENUMS=$$(grep 'enum "' $(FIREWALL_SERVICES_YANG) |				\
	         sed 's/.*enum "\([^"]*\)".*/\1/');					\
	MISSING=0;									\
	for service in $$ENUMS; do							\
		if [ ! -f "$(FIREWALL_DAEMON_DIR)/services/$$service.xml" ]; then	\
			echo "Service $$service is not a known firewalld service";	\
			MISSING=1;							\
		fi;									\
	done;										\
	if [ $$MISSING -eq 1 ]; then							\
		exit 1;									\
	fi; 										\
	cd $(FIREWALL_DAEMON_DIR)/services/;						\
	for xmlfile in *.xml; do							\
		service=$${xmlfile%.xml};						\
		if ! echo "$$ENUMS" | grep -q "^$$service$$"; then			\
			rm "$$xmlfile";							\
		fi;									\
	done
endef

define FIREWALL_MARK_BUILTINS
	for xmlfile in $(FIREWALL_XML_FILES); do					\
		[ -f "$$xmlfile" ] || continue;						\
		grep -q "(immutable)" "$$xmlfile" && continue;				\
		if grep -q '<short>' "$$xmlfile"; then					\
			sed -i 's|<short>\(.*\)</short>|<short>\1 (immutable)</short>|'	\
				"$$xmlfile";						\
		else									\
			if echo "$$xmlfile" | grep -q "/policies/"; then		\
				sed -i 's|<policy|<short>(immutable)</short>\n&|'	\
					"$$xmlfile";					\
			else								\
				sed -i 's|<zone|<short>(immutable)</short>\n&|'		\
					"$$xmlfile";					\
			fi;								\
		fi;									\
	done
endef

FIREWALL_TARGET_FINALIZE_HOOKS += FIREWALL_CLEANUP
FIREWALL_TARGET_FINALIZE_HOOKS += FIREWALL_PRUNE_SERVICES
FIREWALL_TARGET_FINALIZE_HOOKS += FIREWALL_MARK_BUILTINS

$(eval $(generic-package))
