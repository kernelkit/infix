################################################################################
#
# TSN shapers
#
################################################################################

FEATURE_TSN_PACKAGE_VERSION = 1.0
FEATURE_TSN_PACKAGE_LICENSE = MIT

define FEATURE_TSN_LINUX_CONFIG_FIXUPS
	$(call KCONFIG_ENABLE_OPT,CONFIG_NET_SCH_CBS)
	$(call KCONFIG_ENABLE_OPT,CONFIG_NET_SCH_TAPRIO)
endef

$(eval $(generic-package))
