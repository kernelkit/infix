
.PHONY: board-enable-sparx-fit
board-enable-sparx-fit:
	@echo "Enabling SparX-5i compatible FIT options"
	@BR2_PREFIX= ./utils/config --file $(BR2_CONFIG) \
	 	--enable FIT_IMAGE \
	 	--set-str FIT_KERNEL_LOAD_ADDR "0x7 0x00000000"
