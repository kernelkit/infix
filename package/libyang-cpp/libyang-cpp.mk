################################################################################
#
# klish
#
################################################################################
LIBYANG_CPP_VERSION = 5884eeaa79c41ff2bc00c8458f30033ac37c67a3
LIBYANG_CPP_SITE = git@github.com:CESNET/libyang-cpp.git
LIBYANG_CPP_SITE_METHOD = git
LIBYANG_CPP_LICENSE = BSD-3-Clause
LIBYANG_CPP_LICENSE_FILES = LICENCE
LIBYANG_CPP_DEPENDENCIES = libyang
LIBYANG_CPP_INSTALL_STAGING = YES
LIBYANG_CPP_AUTORECONF = YES

$(eval $(cmake-package))
