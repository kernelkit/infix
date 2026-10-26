################################################################################
#
# Rousette RESTconf server
#
################################################################################
ROUSETTE_VERSION = b3bcfd9ec22d4a50481912fdf4ca4b19cf615078
ROUSETTE_SITE = https://github.com/kernelkit/rousette.git
ROUSETTE_SITE_METHOD = git
ROUSETTE_LICENSE = Apache-2.0
ROUSETTE_LICENSE_FILES = LICENSE
ROUSETTE_DEPENDENCIES = boost docopt-cpp nghttp2-asio spdlog sysrepo-cpp linux-pam date-cpp

ROUSETTE_CONF_OPTS = \
	-DTESTAR=on \
	-DTHREADS_PTHREAD_ARG:STRING=-pthread

define ROUSETTE_USERS
	yangnobody 333666 yangnobody 333666 * - - - Unauthenticated operations via RESTCONF
endef
$(eval $(cmake-package))
