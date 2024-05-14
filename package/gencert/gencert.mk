################################################################################
#
# gencert
#
################################################################################

GENCERT_VERSION = 1.0
GENCERT_SITE_METHOD = local
GENCERT_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/gencert
GENCERT_LICENSE = ISC
GENCERT_LICENSE_FILES = LICENSE
GENCERT_REDISTRIBUTE = NO
GENCERT_DEPENDENCIES = host-pkgconf libopenssl
GENCERT_AUTORECONF = YES

define GENCERT_CONF_ENV
CFLAGS="$(INFIX_CFLAGS)"
endef

GENCERT_CONF_OPTS = --prefix= --disable-silent-rules

$(eval $(autotools-package))
