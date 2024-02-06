################################################################################
#
# libnghttp2-asio
#
################################################################################

LIBNGHTTP_ASIO_VERSION = e877868abe06a83ed0a6ac6e245c07f6f20866b5
LIBNGHTTP_ASIO_SITE = git@github.com:nghttp2/nghttp2-asio.git
LIBNGHTTP_ASIO_SITE_METHOD = git
LIBNGHTTP_ASIO_LICENSE = BSD-3
LIBNGHTTP_ASIO_LICENSE_FILES = LICENCE
LIBNGHTTP_ASIO_DEPENDENCIES = boost
LIBNGHTTP_ASIO_INSTALL_STAGING = YES
LIBNGHTTP_ASIO_AUTOGEN = YES
LIBNGHTTP_ASIO_AUTORECONF = YES

$(eval $(cmake-package))
