################################################################################
#
# umgmt
#
################################################################################

UMGMT_VERSION = b347e8e0f9ca97da94440ae2381920aface42e60
UMGMT_SITE = https://github.com/sartura/umgmt
UMGMT_SITE_METHOD = git
UMGMT_GIT_SUBMODULES = YES
UMGMT_INSTALL_STAGING = YES

UMGMT_LICENSE = BSD-3
UMGMT_LICENSE_FILES = LICENSE

$(eval $(cmake-package))
