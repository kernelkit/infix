################################################################################
#
# CPP bindings for sysrepo
#
################################################################################

SYSREPO_CPP_VERSION = 99747f74e57a09c664251ea1f4e059e3f4f8e66a
SYSREPO_CPP_SITE = https://github.com/kernelkit/sysrepo-cpp.git
SYSREPO_CPP_SITE_METHOD = git
SYSREPO_CPP_LICENSE = BSD-3-Clause
SYSREPO_CPP_LICENSE_FILES = LICENSE
SYSREPO_CPP_DEPENDENCIES = sysrepo libyang-cpp
SYSREPO_CPP_INSTALL_STAGING = YES

$(eval $(cmake-package))
