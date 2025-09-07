################################################################################
#
# bootloader-splashscreen
#
################################################################################

BOOTLOADER_SPLASHSCREEN_VERSION = 1.0
BOOTLOADER_SPLASHSCREEN_SOURCE =
BOOTLOADER_SPLASHSCREEN_SITE =

define BOOTLOADER_SPLASHSCREEN_BUILD_CMDS
	# Nothing to build
endef

define BOOTLOADER_SPLASHSCREEN_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0644 $(call qstrip,$(BR2_PACKAGE_BOOTLOADER_SPLASHSCREEN_PATH)) $(BINARIES_DIR)/splash.bmp
endef

$(eval $(generic-package))
