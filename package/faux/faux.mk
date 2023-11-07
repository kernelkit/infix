################################################################################
#
# faux
#
################################################################################

FAUX_VERSION = df1d569287bc45d8fd880287c00e3874c5627c19
FAUX_SITE = https://github.com/kernelkit/faux.git
#FAUX_VERSION = tags/2.1.0
#FAUX_SITE = https://src.libcode.org/pkun/faux.git
FAUX_SITE_METHOD = git
FAUX_LICENSE = BSD-3
FAUX_LICENSE_FILES = LICENCE
FAUX_INSTALL_STAGING = YES
FAUX_AUTORECONF = YES

$(eval $(autotools-package))
