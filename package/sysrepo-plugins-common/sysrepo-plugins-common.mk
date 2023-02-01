################################################################################
#
# sysrepo-plugins-common
#
################################################################################

SYSREPO_PLUGINS_COMMON_VERSION = 20885de0d3bb95a05610fdb3a0f83d8f7c370fad
SYSREPO_PLUGINS_COMMON_SITE = https://github.com/telekom/sysrepo-plugins-common
SYSREPO_PLUGINS_COMMON_SITE_METHOD = git
SYSREPO_PLUGINS_COMMON_GIT_SUBMODULES = YES
SYSREPO_PLUGINS_COMMON_LICENSE = BSD-3
SYSREPO_PLUGINS_COMMON_LICENSE_FILES = LICENSE
SYSREPO_PLUGINS_COMMON_INSTALL_STAGING = YES
SYSREPO_PLUGINS_COMMON_DEPENDENCIES = libyang host-sysrepo sysrepo

ifeq ($(BR2_INIT_SYSTEMD),y)
SYSREPO_PLUGINS_COMMON_DEPENDENCIES += systemd
else

endif

$(eval $(cmake-package))
