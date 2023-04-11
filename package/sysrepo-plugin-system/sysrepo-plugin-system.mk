################################################################################
#
# sysrepo-plugin-system
#
################################################################################

SYSREPO_PLUGIN_SYSTEM_VERSION = 2fe1abe6def24697e639606eeaee5ed0833d3855
SYSREPO_PLUGIN_SYSTEM_SITE = https://github.com/telekom/sysrepo-plugin-system
SYSREPO_PLUGIN_SYSTEM_SITE_METHOD = git
SYSREPO_PLUGIN_SYSTEM_GIT_SUBMODULES = YES
SYSREPO_PLUGIN_SYSTEM_LICENSE = BSD-3
SYSREPO_PLUGIN_SYSTEM_LICENSE_FILES = LICENSE
SYSREPO_PLUGIN_SYSTEM_INSTALL_STAGING = YES
SYSREPO_PLUGIN_SYSTEM_DEPENDENCIES = libyang host-sysrepo sysrepo sysrepo-plugins-common umgmt

ifeq ($(BR2_INIT_SYSTEMD),y)
SYSREPO_PLUGINS_COMMON_DEPENDENCIES += systemd
endif

define SYSREPO_PLUGIN_SYSTEM_INSTALL_YANG_MODELS
	$(INSTALL) -D -m 0644 $(@D)/yang/iana-crypt-hash@2014-08-06.yang \
                $(TARGET_DIR)/usr/share/yang/modules/sysrepo-plugin-system/iana-crypt-hash@2014-08-06.yang
	$(INSTALL) -D -m 0644 $(@D)/yang/ietf-system@2014-08-06.yang \
                $(TARGET_DIR)/usr/share/yang/modules/sysrepo-plugin-system/ietf-system@2014-08-06.yang
	$(INSTALL) -D -m 0644 $(@D)/yang/infix-system@2014-08-06.yang \
                $(TARGET_DIR)/usr/share/yang/modules/sysrepo-plugin-system/infix-system@2014-08-06.yang
endef
SYSREPO_PLUGIN_SYSTEM_POST_INSTALL_TARGET_HOOKS += SYSREPO_PLUGIN_SYSTEM_INSTALL_YANG_MODELS

$(eval $(cmake-package))
