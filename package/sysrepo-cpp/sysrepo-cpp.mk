################################################################################
#
# klish
#
################################################################################

SYSREPO_CPP_VERSION = e7f2b3b5efd80d1209a9de2f7976db099d25cc9f
SYSREPO_CPP_SITE = git@github.com:sysrepo/sysrepo-cpp.git
SYSREPO_CPP_SITE_METHOD = git
SYSREPO_CPP_LICENSE = BSD-3-Clause
SYSREPO_CPP_LICENSE_FILES = LICENCE
SYSREPO_CPP_DEPENDENCIES = sysrepo libyang-cpp
SYSREPO_CPP_INSTALL_STAGING = YES

$(eval $(cmake-package))
