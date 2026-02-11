################################################################################
#
# GPS/GNSS support
#
################################################################################

FEATURE_GPS_PACKAGE_VERSION = 1.0
FEATURE_GPS_PACKAGE_LICENSE = MIT
FEATURE_GPS_DEPENDENCIES +=  gpsd

define FEATURE_GPS_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0644 $(FEATURE_GPS_PKGDIR)/gpsd.default \
		$(TARGET_DIR)/etc/default/gpsd
endef

# Install custom udev rules as a gpsd post-install hook to ensure
# they are not overwritten by gpsd during parallel or incremental builds.
define FEATURE_GPS_INSTALL_UDEV_RULES
	$(INSTALL) -D -m 0644 $(FEATURE_GPS_PKGDIR)/25-gpsd.rules \
		$(TARGET_DIR)/usr/lib/udev/rules.d/25-gpsd.rules
endef
GPSD_POST_INSTALL_TARGET_HOOKS += FEATURE_GPS_INSTALL_UDEV_RULES

define FEATURE_GPS_LINUX_CONFIG_FIXUPS
	$(call KCONFIG_SET_OPT,CONFIG_USB_ACM,m)
endef

$(eval $(generic-package))
