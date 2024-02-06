################################################################################
#
# klish
#
################################################################################

ROUSETTE_VERSION = If9f45e9f347af11b8c6cea2e1094f26318a93e46
ROUSETTE_SITE = git@github.com:CESNET/rousette.git
ROUSETTE_SITE_METHOD = git
ROUSETTE_LICENSE = Apache
ROUSETTE_LICENSE_FILES = LICENCE
ROUSETTE_DEPENDENCIES = libnghttp2-asio libyang-cpp sysrepo-cpp spdlog
ROUSETTE_INSTALL_STAGING = YES

$(eval $(cmake-package))

