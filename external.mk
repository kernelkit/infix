include $(BR2_EXTERNAL_INFIX_PATH)/infix.mk
include $(sort $(wildcard $(BR2_EXTERNAL_INFIX_PATH)/package/*/*.mk))
include $(BR2_EXTERNAL_INFIX_PATH)/board/common/common.mk
-include $(BR2_EXTERNAL_INFIX_PATH)/board/$(patsubst "%",%,$(BR2_ARCH))/board.mk
include $(BR2_EXTERNAL_INFIX_PATH)/test/test.mk

.PHONY: local.mk
local.mk:
	@$(call IXMSG,"Installing local override for certain packages")
	@(cd $O && ln -s $(BR2_EXTERNAL_INFIX_PATH)/local.mk .)

.PHONY: run
run:
	@$(BINARIES_DIR)/qemu.sh

.PHONY: run-menuconfig
run-menuconfig: $(BUILD_DIR)/buildroot-config/mconf
	CONFIG_="CONFIG_" BR2_CONFIG="$(BINARIES_DIR)/.config" \
		$(BUILD_DIR)/buildroot-config/mconf $(BINARIES_DIR)/Config.in

#
# Buildroot package extensions
#
define FRR_POST_BUILD_HOOK
	mkdir -p $(TARGET_DIR)/etc/iproute2/
	cp -r $(@D)/tools/etc/iproute2/rt_protos.d/ $(TARGET_DIR)/etc/iproute2/
endef

FRR_POST_BUILD_HOOKS += FRR_POST_BUILD_HOOK

#
# External pre-built toolchains do not carry their own license.
#
# The Bootlin toolchains used by Infix are built from Buildroot and
# compose a .csv file of all components included in the toolchain.
#
define TOOLCHAIN_BOOTLIN_POST_HOOK
	cp $(TOOLCHAIN_EXTERNAL_DOWNLOAD_INSTALL_DIR)/summary.csv \
		$(LEGAL_INFO_DIR)/toolchain-external-bootlin.csv
endef

TOOLCHAIN_EXTERNAL_BOOTLIN_POST_LEGAL_INFO_HOOKS += TOOLCHAIN_BOOTLIN_POST_HOOK
