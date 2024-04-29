define uboot-add-pubkey
	$(call IXMSG,"Installing trusted key $1")
	$(HOST_DIR)/bin/fdt_add_pubkey \
		-a sha256,rsa$(shell \
			openssl x509 -text -noout -in $1 | \
			grep 'Public-Key: ' | \
			sed -e 's/.*(\(.*\) bit)/\1/') \
		-k $(dir $1) \
		-n $(notdir $(basename $1)) \
		-r image \
		$2
endef

# U-Boot has its own public key format which has to be stored in its
# control DT. So we collect all of the trusted keys, convert them to
# the required format, and write the result to infix-key.dtb in the
# U-Boot build tree. This will then be built in to the final U-Boot
# image's control DT via the CONFIG_DEVICE_TREE_INCLUDES option (see
# extras.config).
define UBOOT_PRE_BUILD_INSTALL_KEY
	$(HOST_DIR)/bin/dtc <(echo '/dts-v1/; / { signature {}; };') >$(@D)/infix-key.dtb
	$(foreach key, \
		$(call qstrip,$(TRUSTED_KEYS_DEVELOPMENT_PATH)) $(call qstrip,$(TRUSTED_KEYS_EXTRA_PATH)),\
		$(call uboot-add-pubkey,$(key),$(@D)/infix-key.dtb))
	$(HOST_DIR)/bin/dtc -I dtb -O dts \
		<$(@D)/infix-key.dtb \
	| sed -e 's:/dts-v[0-9]\+/;::' >$(@D)/arch/$(UBOOT_ARCH)/dts/infix-key.dtsi
	rm $(@D)/infix-key.dtb
endef
UBOOT_PRE_BUILD_HOOKS += UBOOT_PRE_BUILD_INSTALL_KEY

define UBOOT_PRE_BUILD_INSTALL_ENV
	@$(call IXMSG,"Installing Infix environment extensions")
	cp -f $(BR2_EXTERNAL_INFIX_PATH)/board/common/uboot/env.dtsi \
		$(@D)/arch/$(UBOOT_ARCH)/dts/infix-env.dtsi
	cp -af $(BR2_EXTERNAL_INFIX_PATH)/board/common/uboot/scripts \
		$(@D)/arch/$(UBOOT_ARCH)/dts/
endef
UBOOT_PRE_BUILD_HOOKS += UBOOT_PRE_BUILD_INSTALL_ENV
