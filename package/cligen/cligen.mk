################################################################################
#
# cligen
#
################################################################################

CLIGEN_VERSION = 6.1.0
CLIGEN_SITE = $(call github,clicon,cligen,$(CLIGEN_VERSION))
CLIGEN_LICENSE = Apache-2.0
CLIGEN_LICENSE_FILES = LICENSE.md
CLIGEN_INSTALL_STAGING = YES
CLIGEN_CONF_OPTS = INSTALLFLAGS=" "

$(eval $(autotools-package))
