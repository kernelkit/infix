################################################################################
#
# nghttp2-asio
#
################################################################################

NGHTTP2_ASIO_VERSION = e877868abe06a83ed0a6ac6e245c07f6f20866b5
NGHTTP2_ASIO_SITE = https://github.com/kernelkit/nghttp2-asio.git
NGHTTP2_ASIO_SITE_METHOD = git
NGHTTP2_ASIO_LICENSE = MIT
NGHTTP2_ASIO_LICENSE_FILES = LICENCE
NGHTTP2_ASIO_DEPENDENCIES = boost nghttp2
NGHTTP2_ASIO_INSTALL_STAGING = YES
NGHTTP2_ASIO_AUTOGEN = YES
NGHTTP2_ASIO_AUTORECONF = YES

$(eval $(cmake-package))
