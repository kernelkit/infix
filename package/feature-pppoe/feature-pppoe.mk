################################################################################
#
# PPPoE client
#
################################################################################

FEATURE_PPPOE_PACKAGE_VERSION = 1.0
FEATURE_PPPOE_PACKAGE_LICENSE = MIT

define FEATURE_PPPOE_LINUX_CONFIG_FIXUPS
	$(call KCONFIG_SET_OPT,CONFIG_PPP,m)
	$(call KCONFIG_SET_OPT,CONFIG_PPPOE,m)
endef

$(eval $(generic-package))
