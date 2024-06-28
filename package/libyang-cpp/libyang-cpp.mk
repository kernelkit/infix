################################################################################
#
# CPP bindings for libyang
#
################################################################################
LIBYANG_CPP_VERSION = b852ea3b9a2729da364f2c4122db05d882df37f2
LIBYANG_CPP_SITE = https://github.com/kernelkit/libyang-cpp.git
LIBYANG_CPP_SITE_METHOD = git
LIBYANG_CPP_LICENSE = BSD-3-Clause
LIBYANG_CPP_LICENSE_FILES = LICENSE
LIBYANG_CPP_DEPENDENCIES = libyang
LIBYANG_CPP_INSTALL_STAGING = YES

$(eval $(cmake-package))
