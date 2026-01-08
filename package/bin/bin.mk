################################################################################
#
# bin
#
################################################################################

BIN_VERSION = 1.0
BIN_SITE_METHOD = local
BIN_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/bin
BIN_LICENSE = BSD-3-Clause
BIN_LICENSE_FILES = LICENSE
BIN_REDISTRIBUTE = NO
BIN_DEPENDENCIES = sysrepo libite
BIN_CONF_OPTS = --disable-silent-rules
BIN_AUTORECONF = YES

define BIN_CONF_ENV
CFLAGS="$(INFIX_CFLAGS)"
endef

define BIN_PERMISSIONS
	/usr/bin/copy  d 04750 root sysrepo - - - - -
endef

$(eval $(autotools-package))
