################################################################################
#
# GPS/GNSS support
#
################################################################################

FEATURE_GPS_PACKAGE_VERSION = 1.0
FEATURE_GPS_PACKAGE_LICENSE = MIT

define FEATURE_GPS_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0644 $(FEATURE_GPS_PKGDIR)/gpsd.default \
		$(TARGET_DIR)/etc/default/gpsd
	$(INSTALL) -D -m 0644 $(FEATURE_GPS_PKGDIR)/25-gpsd.rules \
		$(TARGET_DIR)/usr/lib/udev/rules.d/25-gpsd.rules
	$(INSTALL) -D -m 0644 $(FEATURE_GPS_PKGDIR)/gpsd.conf \
		$(TARGET_DIR)/etc/finit.d/available/gpsd.conf
endef

define FEATURE_GPS_LINUX_CONFIG_FIXUPS
	$(call KCONFIG_SET_OPT,CONFIG_USB_ACM,m)
endef

$(eval $(generic-package))
