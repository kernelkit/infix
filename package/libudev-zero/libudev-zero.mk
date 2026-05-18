################################################################################
#
# libudev-zero
#
################################################################################

LIBUDEV_ZERO_VERSION = 1.0.3
LIBUDEV_ZERO_SITE = $(call github,illiliti,libudev-zero,$(LIBUDEV_ZERO_VERSION))
LIBUDEV_ZERO_LICENSE = ISC
LIBUDEV_ZERO_LICENSE_FILES = LICENSE
LIBUDEV_ZERO_INSTALL_STAGING = YES
LIBUDEV_ZERO_PROVIDES = udev

define LIBUDEV_ZERO_BUILD_CMDS
	$(MAKE) -C $(@D) CC="$(TARGET_CC)" AR="$(TARGET_AR)" \
		CFLAGS="$(TARGET_CFLAGS)" LDFLAGS="$(TARGET_LDFLAGS)" \
		PREFIX=/usr
endef

define LIBUDEV_ZERO_INSTALL_STAGING_CMDS
	$(MAKE) -C $(@D) DESTDIR=$(STAGING_DIR) PREFIX=/usr install
endef

define LIBUDEV_ZERO_INSTALL_TARGET_CMDS
	$(MAKE) -C $(@D) DESTDIR=$(TARGET_DIR) PREFIX=/usr install
endef

$(eval $(generic-package))
