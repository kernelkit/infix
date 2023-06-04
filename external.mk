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
	@(cd $O/images && ./qemu.sh)

.PHONY: run-menuconfig
run-menuconfig:
	@(cd $O/images && CONFIG_="CONFIG_" BR2_CONFIG=".config" ../build/buildroot-config/mconf Config.in)
