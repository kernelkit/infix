################################################################################
#
# klinfix
#
################################################################################

KLINFIX_VERSION = 1.0
KLINFIX_LICENSE = BSD-3-Clause
KLINFIX_SITE_METHOD = local
KLINFIX_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/klinfix
KLINFIX_DEPENDENCIES = klish-plugin-sysrepo
KLINFIX_AUTORECONF = YES

$(eval $(autotools-package))
