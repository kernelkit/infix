################################################################################
#
# klish-plugin-sysrepo
#
################################################################################

KLISH_PLUGIN_SYSREPO_VERSION = 3b4e69780e64f0b8f42dc3784defe72285911ad4
KLISH_PLUGIN_SYSREPO_SITE = https://github.com/kernelkit/klish-plugin-sysrepo.git
KLISH_PLUGIN_SYSREPO_SITE_METHOD = git
KLISH_PLUGIN_SYSREPO_LICENSE = BSD-3
KLISH_PLUGIN_SYSREPO_LICENSE_FILES = LICENCE
KLISH_PLUGIN_SYSREPO_DEPENDENCIES = klish sysrepo
KLISH_PLUGIN_SYSREPO_INSTALL_STAGING = YES
KLISH_PLUGIN_SYSREPO_AUTORECONF = YES

ifeq ($(BR2_PACKAGE_KLISH_PLUGIN_SYSREPO_XML),y)
define KLISH_PLUGIN_SYSREPO_INSTALL_XML
	$(INSTALL) -t $(TARGET_DIR)/etc/klish -D -m 0644 $(@D)/xml/sysrepo.xml
endef
KLISH_PLUGIN_SYSREPO_POST_INSTALL_TARGET_HOOKS += \
	KLISH_PLUGIN_SYSREPO_INSTALL_XML
endif

$(eval $(autotools-package))
