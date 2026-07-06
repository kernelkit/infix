include $(BR2_EXTERNAL_INFIX_PATH)/board/common/image/image.mk
include $(BR2_EXTERNAL_INFIX_PATH)/board/common/qemu/qemu.mk

ifeq ($(IX_TRUSTED_KEYS),y)
include $(BR2_EXTERNAL_INFIX_PATH)/board/common/uboot/uboot.mk

IX_TRUSTED_KEYS=$(IX_TRUSTED_KEYS_DEVELOPMENT_PATH) $(IX_TRUSTED_KEYS_EXTRA_PATH)
define RAUC_POST_BUILD_INSTALL_CERT
	@$(call IXMSG,"Installing signing cert for RAUC")
	mkdir -p $(TARGET_DIR)/etc/rauc/keys
	$(foreach crt,$(shell ls $(IX_TRUSTED_KEYS)), \
		cp $(crt) $(TARGET_DIR)/etc/rauc/keys/$(shell openssl x509 -hash -noout <$(crt)).0;)

endef
RAUC_POST_BUILD_HOOKS += RAUC_POST_BUILD_INSTALL_CERT
endif
