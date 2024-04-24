################################################################################
#
# libsrx
#
################################################################################

LIBSRX_VERSION = 1.0.0
LIBSRX_SITE_METHOD = local
LIBSRX_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/libsrx
LIBSRX_LICENSE = BSD-3-Clause
LIBSRX_LICENSE_FILES = LICENSE
LIBSRX_INSTALL_STAGING = YES
LIBSRX_REDISTRIBUTE = NO
LIBSRX_DEPENDENCIES = sysrepo libite
LIBSRX_AUTORECONF = YES
define LIBSRX_CONF_ENV
CFLAGS="$(INFIX_CFLAGS)"
endef
$(eval $(autotools-package))
