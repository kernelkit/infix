################################################################################
#
# CPP bindings for libyang
#
################################################################################
LIBYANG_CPP_VERSION = v3
LIBYANG_CPP_SITE = $(call github,CESNET,libyang-cpp,$(LIBYANG_CPP_VERSION))
LIBYANG_CPP_LICENSE = BSD-3-Clause
LIBYANG_CPP_LICENSE_FILES = LICENSE
LIBYANG_CPP_DEPENDENCIES = libyang
LIBYANG_CPP_INSTALL_STAGING = YES

$(eval $(cmake-package))
