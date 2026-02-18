################################################################################
#
# nghttp2-asio
#
################################################################################

NGHTTP2_ASIO_VERSION = 2173b82e6caa85950c769eecc5da6809faadf61a
NGHTTP2_ASIO_SITE = https://github.com/CESNET/nghttp2-asio
NGHTTP2_ASIO_SITE_METHOD = git
NGHTTP2_ASIO_LICENSE = MIT
NGHTTP2_ASIO_LICENSE_FILES = COPYING
NGHTTP2_ASIO_DEPENDENCIES = boost nghttp2 openssl
NGHTTP2_ASIO_INSTALL_STAGING = YES
NGHTTP2_ASIO_AUTOGEN = YES
NGHTTP2_ASIO_AUTORECONF = YES
NGHTTP2_ASIO_CONF_OPTS += -DCMAKE_POLICY_VERSION_MINIMUM=3.5

$(eval $(cmake-package))
