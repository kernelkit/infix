################################################################################
#
# modules
#
################################################################################

MODULES_VERSION = 1.0
MODULES_SITE_METHOD = local
MODULES_SITE = $(BR2_EXTERNAL_INFIX_PATH)/modules
MODULES_LICENSE = GPLv2
MODULES_LICENSE_FILES = COPYING
MODULES_REDISTRIBUTE = NO
MODULES_DEPENDENCIES = linux

MODULES_VARS := $(foreach var,$(.VARIABLES),$(filter BR2_PACKAGE_MODULES_%,$(var)))

define MODULES_CONFIGURE_CMDS
	: >$(@D)/.config
	$(foreach mod,$(MODULES_VARS),echo MOD_$(mod:BR2_PACKAGE_MODULES_%=%)=m >>$(@D)/.config; )
endef

ifeq ($(BR2_PACKAGE_MODULES_NET_DSA_PASW_TOOL),y)
define INSTALL_PASW_TOOL
	$(INSTALL) -D -m 0755 $(@D)/drivers/net/dsa/pasw/tools/pasw \
		$(TARGET_DIR)/usr/sbin/pasw
endef
MODULES_POST_INSTALL_TARGET_HOOKS += INSTALL_PASW_TOOL
endif

define MODULES_LINUX_CONFIG_FIXUPS
#       Selected to implicily select CONFIG_REGMAP_MMIO
	$(if $(BR2_PACKAGE_MODULES_PTP_PG210_PTP),$(call KCONFIG_ENABLE_OPT,CONFIG_MFD_SYSCON))

	$(if $(BR2_PACKAGE_MODULES_NET_DSA_PASW),$(call KCONFIG_ENABLE_OPT,CONFIG_COMMON_CLK))
	$(if $(BR2_PACKAGE_MODULES_NET_DSA_PASW),$(call KCONFIG_ENABLE_OPT,CONFIG_NET_DSA))
	$(if $(BR2_PACKAGE_MODULES_NET_DSA_PASW),$(call KCONFIG_ENABLE_OPT,CONFIG_SFP))
	$(if $(BR2_PACKAGE_MODULES_NET_DSA_PASW_TOOL),$(call KCONFIG_DISABLE_OPT,CONFIG_STRICT_DEVMEM))
endef

$(eval $(kernel-module))
$(eval $(generic-package))
