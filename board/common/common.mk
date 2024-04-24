ifeq ($(SIGN_ENABLED),y)
include $(BR2_EXTERNAL_INFIX_PATH)/board/common/uboot/uboot.mk

TRUSTED_KEYS=$(TRUSTED_KEYS_DEVELOPMENT_PATH) $(TRUSTED_KEYS_EXTRA_PATH)
define RAUC_POST_BUILD_INSTALL_CERT
	@$(call IXMSG,"Installing signing cert for RAUC")
	mkdir -p $(TARGET_DIR)/etc/rauc/keys
	$(foreach crt,$(shell ls $(TRUSTED_KEYS)), \
		cp $(crt) $(TARGET_DIR)/etc/rauc/keys/$(shell openssl x509 -hash -noout <$(crt)).0;)

endef
RAUC_POST_BUILD_HOOKS += RAUC_POST_BUILD_INSTALL_CERT
endif
