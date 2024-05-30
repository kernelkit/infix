################################################################################
#
# Rousette RESTconf server
#
################################################################################

ROUSETTE_VERSION = 226a2410376e5befd0bf6dc180618a108b11b256
ROUSETTE_SITE = https://github.com/kernelkit/rousette.git
ROUSETTE_SITE_METHOD = git
ROUSETTE_LICENSE = Apache-2.0
ROUSETTE_LICENSE_FILES = LICENCE
ROUSETTE_DEPENDENCIES = boost docopt-cpp nghttp2-asio spdlog sysrepo-cpp linux-pam

ROUSETTE_CONF_OPTS = \
	-DTESTAR=on \
	-DTHREADS_PTHREAD_ARG:STRING=-pthread

define ROUSETTE_USERS
	yangnobody 333666 yangnobody 333666 * - - - Unauthenticated operations via RESTCONF
endef
$(eval $(cmake-package))

