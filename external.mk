include $(sort $(wildcard $(BR2_EXTERNAL_INFIX_PATH)/package/*/*.mk))

.PHONY: run
run:
	@echo "Starting Qemu  ::  Ctrl-a x -- exit | Ctrl-a c -- toggle console/monitor"
	@$(BR2_EXTERNAL_INFIX_PATH)/qemu/qemu.sh $O
