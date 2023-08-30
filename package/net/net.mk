################################################################################
#
# net
#
################################################################################

NET_VERSION = 1.0
NET_LICENSE = MIT
NET_LICENSE_FILES = LICENSE
NET_SITE_METHOD = local
NET_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/net
NET_REDISTRIBUTE = NO
NET_DEPENDENCIES = libite
NET_AUTORECONF = YES

define NET_INSTALL_EXTRA
	chmod +x $(TARGET_DIR)/usr/share/net/*.sh
endef
NET_POST_INSTALL_TARGET_HOOKS += NET_INSTALL_EXTRA

$(eval $(autotools-package))
