################################################################################
#
# CPP bindings for sysrepo
#
################################################################################

SYSREPO_CPP_VERSION = e59193c772fa0e5963eff216b4fbb574383a64f2
SYSREPO_CPP_SITE = https://github.com/kernelkit/sysrepo-cpp.git
SYSREPO_CPP_SITE_METHOD = git
SYSREPO_CPP_LICENSE = BSD-3-Clause
SYSREPO_CPP_LICENSE_FILES = LICENSE
SYSREPO_CPP_DEPENDENCIES = sysrepo libyang-cpp
SYSREPO_CPP_INSTALL_STAGING = YES

$(eval $(cmake-package))
