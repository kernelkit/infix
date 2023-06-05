IXMSG = printf "\e[37;44m>>>   $(call qstrip,$(1))\e[0m\n"

include $(sort $(wildcard $(BR2_EXTERNAL_INFIX_PATH)/package/*/*.mk))
include $(BR2_EXTERNAL_INFIX_PATH)/board/common/common.mk
-include $(BR2_EXTERNAL_INFIX_PATH)/board/$(patsubst "%",%,$(BR2_ARCH))/board.mk

.PHONY: local.mk
local.mk:
	@$(call IXMSG,"Installing local override for certain packages")
	@(cd $O && ln -s $(BR2_EXTERNAL_INFIX_PATH)/local.mk .)

.PHONY: run
run:
	@$(BINARIES_DIR)/qemu.sh

.PHONY: run-menuconfig
run-menuconfig:
	CONFIG_="CONFIG_" BR2_CONFIG="$(BINARIES_DIR)/.config" \
		$(BUILD_DIR)/buildroot-config/mconf $(BINARIES_DIR)/Config.in
