################################################################################
#
# klish-plugin-infix
#
################################################################################

KLISH_PLUGIN_INFIX_VERSION = 1.0
KLISH_PLUGIN_INFIX_LICENSE = BSD-3-Clause
KLISH_PLUGIN_INFIX_LICENSE_FILES = LICENSE
KLISH_PLUGIN_INFIX_SITE_METHOD = local
KLISH_PLUGIN_INFIX_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/klish-plugin-infix
KLISH_PLUGIN_INFIX_REDISTRIBUTE = NO
KLISH_PLUGIN_INFIX_DEPENDENCIES = klish-plugin-sysrepo
KLISH_PLUGIN_INFIX_AUTORECONF = YES

define KLISH_PLUGIN_INFIX_INSTALL_DOC
	$(INSTALL) -t $(TARGET_DIR)/usr/share/infix/cli -D -m 0644 \
		$(wildcard $(BR2_EXTERNAL_INFIX_PATH)/doc/cli/*.md)
endef
KLISH_PLUGIN_INFIX_POST_INSTALL_TARGET_HOOKS += KLISH_PLUGIN_INFIX_INSTALL_DOC

$(eval $(autotools-package))
