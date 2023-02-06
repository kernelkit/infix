include $(BR2_EXTERNAL_INFIX_PATH)/board/common/uboot/uboot.mk

define RAUC_POST_BUILD_INSTALL_CERT
	@$(call IXMSG,"Installing signing cert for RAUC")
	mkdir -p $(TARGET_DIR)/etc/rauc/keys
	$(foreach crt,$(shell ls $(SIGN_KEY)/*.crt), \
		cp $(crt) $(TARGET_DIR)/etc/rauc/keys/$(shell openssl x509 -hash -noout <$(crt)).0;)
endef
RAUC_POST_BUILD_HOOKS += RAUC_POST_BUILD_INSTALL_CERT
