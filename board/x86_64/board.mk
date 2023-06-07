test-dir = $(BR2_EXTERNAL_INFIX_PATH)/test

.PHONY: infix-check
infix-check:
	$(test-dir)/env \
		-q $(test-dir)/virt/dual \
		-f $(BINARIES_DIR)/infix-x86_64.img \
		$(BR2_EXTERNAL_INFIX_PATH)/9pm/9pm.py $(test-dir)/case/all.yaml
