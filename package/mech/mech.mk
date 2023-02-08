################################################################################
#
# mech
#
################################################################################
MECH_VERSION = 1.0
MECH_LICENSE = GPL
MECH_SITE_METHOD = local
MECH_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/mech
MECH_AUTORECONF = YES

$(eval $(autotools-package))
