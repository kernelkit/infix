################################################################################
#
# klinfix
#
################################################################################

KLINFIX_VERSION = 1.0
KLINFIX_LICENSE = BSD-3-Clause
KLINFIX_LICENSE_FILES = LICENSE
KLINFIX_SITE_METHOD = local
KLINFIX_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/klinfix
KLINFIX_DEPENDENCIES = klish-plugin-sysrepo
KLINFIX_AUTORECONF = YES

define KLINFIX_INSTALL_DOC
	$(INSTALL) -t $(TARGET_DIR)/usr/share/infix/cli -D -m 0644 \
		$(wildcard $(BR2_EXTERNAL_INFIX_PATH)/doc/cli/*.md)
endef
KLINFIX_POST_INSTALL_TARGET_HOOKS += KLINFIX_INSTALL_DOC

$(eval $(autotools-package))
