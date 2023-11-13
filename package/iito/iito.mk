################################################################################
#
# iito
#
################################################################################

IITO_VERSION = 1.0.0
IITO_SITE = https://github.com/kernelkit/iito/releases/download/v$(IITO_VERSION)
IITO_LICENSE = GPL-2.0
IITO_LICENSE_FILES = COPYING
IITO_DEPENDENCIES = jansson libev udev

$(eval $(autotools-package))
