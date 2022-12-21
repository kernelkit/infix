# This is a bit awkward. If you know about a more straight forward way
# of doing this, please simplify.
#
# U-Boot needs the public part of the signing key to be preprocessed
# and then inserted into its control DT. mkimage(1) can perform this
# conversion, but only as a side-effect of building/signing an FIT
# image. Since we might not always be doing that, e.g. when only
# building a hardware specific bootloader, we build a dummy FIT just
# to get the key information into a DTB, which we then convert back to
# a .dtsi and install in the U-Boot build tree. This will then be
# built in to the final U-Boot image's control DT via the
# CONFIG_DEVICE_TREE_INCLUDES option (see extras.config).
define UBOOT_PRE_BUILD_INSTALL_KEY
	@echo "Installing Infix signing key ($(SIGN_KEY))"
	$(HOST_DIR)/bin/dtc <(echo '/dts-v1/; / { signature {}; };') >$(@D)/infix-key.dtb
	$(HOST_DIR)/bin/mkimage \
		-k $(SIGN_KEY) \
		-f $(BR2_EXTERNAL_INFIX_PATH)/board/common/uboot/key-dummy.its \
		-K $(@D)/infix-key.dtb \
		$(if $(SIGN_SRC_PKCS11),-N pkcs11) \
		-r \
		$(@D)/key-dummy.itb
	rm $(@D)/key-dummy.itb
	$(HOST_DIR)/bin/dtc -I dtb -O dts \
		<$(@D)/infix-key.dtb \
	| sed -e 's:/dts-v[0-9]\+/;::' >$(@D)/arch/$(UBOOT_ARCH)/dts/infix-key.dtsi
	rm $(@D)/infix-key.dtb
endef
UBOOT_PRE_BUILD_HOOKS += UBOOT_PRE_BUILD_INSTALL_KEY

define UBOOT_PRE_BUILD_INSTALL_ENV
	@echo "Installing Infix environment extensions"
	cp -f $(BR2_EXTERNAL_INFIX_PATH)/board/common/uboot/env.dtsi \
		$(@D)/arch/$(UBOOT_ARCH)/dts/infix-env.dtsi
	cp -af $(BR2_EXTERNAL_INFIX_PATH)/board/common/uboot/scripts \
		$(@D)/arch/$(UBOOT_ARCH)/dts/
endef
UBOOT_PRE_BUILD_HOOKS += UBOOT_PRE_BUILD_INSTALL_ENV
