################################################################################
#
# rauc installation status
#
################################################################################

RAUC_INSTALLATION_STATUS_VERSION       = 1.0
RAUC_INSTALLATION_STATUS_LICENSE       = ISC
RAUC_INSTALLATION_STATUS_LICENSE_FILES = LICENSE
RAUC_INSTALLATION_STATUS_SITE_METHOD   = local
RAUC_INSTALLATION_STATUS_SITE          = $(BR2_EXTERNAL_INFIX_PATH)/src/rauc-installation-status
RAUC_INSTALLATION_STATUS_REDISTRIBUTE  = NO
RAUC_INSTALLATION_STATUS_AUTORECONF    = YES
RAUC_INSTALLATION_STATUS_DEPENDENCIES  = host-pkgconf jansson libglib2
RAUC_INSTALLATION_STATUS_CONF_OPTS     = --disable-silent-rules
define RAUC_INSTALLATION_STATUS_CONF_ENV
	CFLAGS="$(INFIX_CFLAGS)"
endef

$(eval $(autotools-package))
