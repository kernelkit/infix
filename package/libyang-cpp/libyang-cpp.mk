################################################################################
#
# CPP bindings for libyang
#
################################################################################
LIBYANG_CPP_VERSION = 38e3399c99a82d3c0f693fb19ab1e1b3cd72aed9
LIBYANG_CPP_SITE = https://github.com/kernelkit/libyang-cpp.git
LIBYANG_CPP_SITE_METHOD = git
LIBYANG_CPP_LICENSE = BSD-3-Clause
LIBYANG_CPP_LICENSE_FILES = LICENSE
LIBYANG_CPP_DEPENDENCIES = libyang
LIBYANG_CPP_INSTALL_STAGING = YES

$(eval $(cmake-package))
