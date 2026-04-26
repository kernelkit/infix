################################################################################
#
# Cellular modem support
#
################################################################################

FEATURE_MODEM_PACKAGE_VERSION = 1.0
FEATURE_MODEM_PACKAGE_LICENSE = MIT

define FEATURE_MODEM_LINUX_CONFIG_FIXUPS
	$(call KCONFIG_ENABLE_OPT,CONFIG_USB_SERIAL)
	$(call KCONFIG_ENABLE_OPT,CONFIG_USB_SERIAL_WWAN)
	$(call KCONFIG_ENABLE_OPT,CONFIG_USB_SERIAL_OPTION)
	$(call KCONFIG_ENABLE_OPT,CONFIG_USB_WDM)
	$(call KCONFIG_ENABLE_OPT,CONFIG_USB_NET_QMI_WWAN)
	$(call KCONFIG_ENABLE_OPT,CONFIG_USB_CDC_MBIM)

	$(if $(filter y,$(BR2_PACKAGE_FEATURE_MODEM_QUALCOMM)),
		$(call KCONFIG_SET_OPT,CONFIG_QRTR,m)
		$(call KCONFIG_SET_OPT,CONFIG_MHI_BUS,m)
		$(call KCONFIG_ENABLE_OPT,CONFIG_QRTR_MHI)
	)
endef

$(eval $(generic-package))
