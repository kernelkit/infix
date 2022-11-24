
.PHONY: board-enable-sparx-fit
board-enable-sparx-fit:
	@echo "Enabling SparX-5i compatible FIT options"
	@BR2_PREFIX= ./utils/config --file $(BR2_CONFIG) \
	 	--enable FIT_IMAGE \
	 	--set-str FIT_KERNEL_LOAD_ADDR "0x7 0x00000000"

.PHONY: board-sparx-flash-uboot
board-sparx-flash-uboot: $(BINARIES_DIR)/u-boot.bin
	@grep -q 'BR2_TARGET_UBOOT_BOARD_DEFCONFIG="mscc_fireant_pcb135_emmc"' $(BR2_CONFIG) || \
		{ echo Build tree does not seem to be configured for SparX-5i eval board; exit 1; }
	@echo Erasing...
	@dangerspi.py erase 0 0x100000
	@echo Programming...
	@dangerspi.py write 0 0x100000 <$<
	@echo Done...
