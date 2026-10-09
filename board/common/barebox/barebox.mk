ifeq ($(IX_IMAGE_BAREBOX_ESP),y)

IX_BAREBOX_DIR   := $(BR2_EXTERNAL_INFIX_PATH)/board/common/barebox
IX_BAREBOX_BUILD := $(BUILD_DIR)/infix-bareboxenv

define BAREBOX_PRE_BUILD_CREATE_ENV
	@$(call IXMSG,"Creating Barebox environment")
	rsync -a $(IX_BAREBOX_DIR)/env/ $(IX_BAREBOX_BUILD)
	dtc <$(IX_BAREBOX_DIR)/state.dts >$(IX_BAREBOX_BUILD)/state.dtb
endef
BAREBOX_PRE_BUILD_HOOKS += BAREBOX_PRE_BUILD_CREATE_ENV

endif
