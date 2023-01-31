################################################################################
#
# ifupdown-ng
#
################################################################################

IFUPDOWN_NG_VERSION = 8c3ba98e5887d6db1586fa58bacdfbf7cf7e3de3
IFUPDOWN_NG_SITE = $(call github,kernelkit,ifupdown-ng,$(IFUPDOWN_NG_VERSION))
#IFUPDOWN_NG_VERSION = 0.12.1
#IFUPDOWN_NG_SITE = $(call github,ifupdown-ng,ifupdown-ng,ifupdown-ng-$(IFUPDOWN_NG_VERSION))
IFUPDOWN_NG_LICENSE = ISC
IFUPDOWN_NG_LICENSE_FILES = COPYING

ifeq ($(BR2_TOOLCHAIN_USES_GLIBC),y)
IFUPDOWN_NG_DEPENDENCIES = host-pkgconf libbsd
FOO = $(shell $(PKG_CONFIG_HOST_BINARY) --cflags libbsd-overlay)
BAR = $(shell $(PKG_CONFIG_HOST_BINARY) --cflags --libs libbsd-overlay)
TARGET_CONFIGURE_OPTS += LIBBSD_CFLAGS="$(FOO)"
TARGET_CONFIGURE_OPTS += LIBBSD_LIBS="$(BAR)"
endif

define IFUPDOWN_NG_BUILD_CMDS
	$(TARGET_MAKE_ENV) $(TARGET_CONFIGURE_OPTS) $(MAKE) \
		LIBBSD_CFLAGS="$(FOO)" LIBBSD_LIBS="$(BAR)"  \
		CFLAGS="$(TARGET_CFLAGS)" -C $(@D) all
endef

# install doesn't overwrite
define IFUPDOWN_NG_INSTALL_TARGET_CMDS
	$(RM) -r $(TARGET_DIR)/usr/libexec/ifupdown-ng
	$(RM) $(TARGET_DIR)/etc/network/ifupdown-ng.conf.example
	$(RM) $(TARGET_DIR)/sbin/{ifupdown,ifup,ifdown,ifquery,ifparse,ifctrstat}
	$(TARGET_MAKE_ENV) $(MAKE) DESTDIR=$(TARGET_DIR) -C $(@D) install
endef

$(eval $(generic-package))
