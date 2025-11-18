include $(sort $(wildcard $(BR2_EXTERNAL_INFIX_PATH)/board/aarch64/*/*.mk))

.PHONY: board-enable-qemu-uboot
board-enable-qemu-uboot:
	@$(call IXMSG,"Enabling build of QEMU compatible U-Boot")
	./utils/config --file $(BR2_CONFIG) \
		--enable PACKAGE_HOST_UBOOT_TOOLS \
		--enable PACKAGE_HOST_UBOOT_TOOLS_FIT_SUPPORT \
		--enable PACKAGE_HOST_UBOOT_TOOLS_FIT_SIGNATURE_SUPPORT \
		--enable TARGET_UBOOT \
		--enable TARGET_UBOOT_BUILD_SYSTEM_KCONFIG \
		--enable TARGET_UBOOT_CUSTOM_VERSION \
		--set-str TARGET_UBOOT_CUSTOM_VERSION_VALUE \
			"2023.04-rc2" \
		--set-str TARGET_UBOOT_PATCH \
			'$$(BR2_EXTERNAL_INFIX_PATH)/board/common/uboot/patches' \
	 	--set-str TARGET_UBOOT_BOARD_DEFCONFIG \
			"qemu_arm64" \
		--set-str TARGET_UBOOT_CONFIG_FRAGMENT_FILES \
			'$$(BR2_EXTERNAL_INFIX_PATH)/board/common/uboot/extras.config' \
		--enable TARGET_UBOOT_FORMAT_DTB

.PHONY: board-sparx-flash-uboot
board-sparx-flash-uboot: $(BINARIES_DIR)/u-boot.bin
	@grep -q 'BR2_TARGET_UBOOT_BOARD_DEFCONFIG="mscc_fireant_pcb135_emmc"' $(BR2_CONFIG) || \
		{ echo Build tree does not seem to be configured for SparX-5i eval board; exit 1; }
	@echo Erasing...
	@dangerspi.py erase 0 0x100000
	@echo Programming...
	@dangerspi.py write 0 0x100000 <$<
	@echo Done...
