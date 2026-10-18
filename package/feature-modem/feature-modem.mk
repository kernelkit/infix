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
	$(call KCONFIG_ENABLE_OPT,CONFIG_USB_NET_CDC_MBIM)
	# AT-only modems carry data as PPP on a tty
	$(call KCONFIG_SET_OPT,CONFIG_PPP_ASYNC,m)

	$(if $(filter y,$(BR2_PACKAGE_FEATURE_MODEM_QUALCOMM)),
		$(call KCONFIG_SET_OPT,CONFIG_QRTR,m)
		$(call KCONFIG_SET_OPT,CONFIG_MHI_BUS,m)
		$(call KCONFIG_ENABLE_OPT,CONFIG_QRTR_MHI)
	)

	# Virtual modem, an ACM gadget looped back by dummy_hcd, which is a
	# module so nodes without a modem have no extra USB bus
	$(if $(filter y,$(BR2_PACKAGE_FEATURE_MODEM_VIRTUAL)),
		$(call KCONFIG_ENABLE_OPT,CONFIG_USB_GADGET)
		$(call KCONFIG_SET_OPT,CONFIG_USB_DUMMY_HCD,m)
		$(call KCONFIG_ENABLE_OPT,CONFIG_CONFIGFS_FS)
		$(call KCONFIG_ENABLE_OPT,CONFIG_USB_CONFIGFS)
		$(call KCONFIG_ENABLE_OPT,CONFIG_USB_CONFIGFS_ACM)
		$(call KCONFIG_ENABLE_OPT,CONFIG_USB_ACM)
	)
endef

ifeq ($(BR2_PACKAGE_FEATURE_MODEM_VIRTUAL),y)
define FEATURE_MODEM_INSTALL_VIRTUAL
	$(INSTALL) -D -m 0755 $(FEATURE_MODEM_PKGDIR)/00-modemsim $(TARGET_DIR)/usr/libexec/infix/init.d/00-modemsim
	$(INSTALL) -D -m 0755 $(FEATURE_MODEM_PKGDIR)/modemsim $(TARGET_DIR)/usr/libexec/infix/modemsim
	$(INSTALL) -D -m 0644 $(FEATURE_MODEM_PKGDIR)/modemsim.conf $(TARGET_DIR)/etc/finit.d/available/modemsim.conf
	ln -sf ../available/modemsim.conf $(TARGET_DIR)/etc/finit.d/enabled/modemsim.conf
endef
FEATURE_MODEM_POST_INSTALL_TARGET_HOOKS += FEATURE_MODEM_INSTALL_VIRTUAL
endif

$(eval $(generic-package))
