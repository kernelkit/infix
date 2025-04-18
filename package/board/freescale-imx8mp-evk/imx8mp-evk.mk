define FREESCALE_IMX8MP_EVK_LINUX_CONFIG_FIXUPS
	$(call KCONFIG_ENABLE_OPT,CONFIG_ARCH_NXP)
	$(call KCONFIG_ENABLE_OPT,CONFIG_ARCH_MXC)
	$(call KCONFIG_ENABLE_OPT,CONFIG_FEC)
	$(call KCONFIG_ENABLE_OPT,CONFIG_NET_VENDOR_STMICRO)
	$(call KCONFIG_ENABLE_OPT,CONFIG_STMMAC_ETH)
	$(call KCONFIG_ENABLE_OPT,CONFIG_DWMAC_IMX8)
	$(call KCONFIG_ENABLE_OPT,CONFIG_SERIAL_IMX)
	$(call KCONFIG_ENABLE_OPT,CONFIG_SERIAL_IMX_CONSOLE)
	$(call KCONFIG_ENABLE_OPT,CONFIG_I2C_IMX)
	$(call KCONFIG_ENABLE_OPT,CONFIG_SPI_IMX)
	$(call KCONFIG_ENABLE_OPT,CONFIG_PINCTRL_IMX8MP)
	$(call KCONFIG_ENABLE_OPT,CONFIG_GPIO_MXC)
	$(call KCONFIG_ENABLE_OPT,CONFIG_IMX2_WDT)
	$(call KCONFIG_ENABLE_OPT,CONFIG_MMC_SDHCI_OF_ESDHC)
	$(call KCONFIG_ENABLE_OPT,CONFIG_MMC_SDHCI_ESDHC_IMX)
	$(call KCONFIG_ENABLE_OPT,CONFIG_CLK_IMX8MP)
	$(call KCONFIG_ENABLE_OPT,CONFIG_NVMEM_IMX_OCOTP)
	$(call KCONFIG_ENABLE_OPT,CONFIG_INTERCONNECT)
	$(call KCONFIG_ENABLE_OPT,CONFIG_INTERCONNECT_IMX)
	$(call KCONFIG_ENABLE_OPT,CONFIG_INTERCONNECT_IMX8MP)
	$(call KCONFIG_ENABLE_OPT,CONFIG_REALTEK_PHY)
endef

$(eval $(ix-board))
$(eval $(generic-package))
