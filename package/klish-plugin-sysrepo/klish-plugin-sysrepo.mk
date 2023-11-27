################################################################################
#
# klish-plugin-sysrepo
#
################################################################################

KLISH_PLUGIN_SYSREPO_VERSION = fa1228a7ff2b3500b8c1bc6f6f0d28bddd2d0a91
KLISH_PLUGIN_SYSREPO_SITE = https://github.com/kernelkit/klish-plugin-sysrepo.git
#KLISH_PLUGIN_SYSREPO_VERSION = cdd3eb51a7f7ee0ed5bd925fa636061d3b1b85fb
#KLISH_PLUGIN_SYSREPO_SITE = https://src.libcode.org/pkun/klish-plugin-sysrepo.git
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
