################################################################################
#
# CPP bindings for sysrepo
#
################################################################################
SYSREPO_CPP_VERSION = v3
SYSREPO_CPP_SITE = $(call github,sysrepo,sysrepo-cpp,$(SYSREPO_CPP_VERSION))
SYSREPO_CPP_LICENSE = BSD-3-Clause
SYSREPO_CPP_LICENSE_FILES = LICENSE
SYSREPO_CPP_DEPENDENCIES = sysrepo libyang-cpp
SYSREPO_CPP_INSTALL_STAGING = YES

$(eval $(cmake-package))
