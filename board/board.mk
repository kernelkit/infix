include $(BR2_EXTERNAL_INFIX_PATH)/board/common/common.mk
include $(BR2_EXTERNAL_INFIX_PATH)/board/ix-board.mk
-include $(BR2_EXTERNAL_INFIX_PATH)/board/$(patsubst "%",%,$(BR2_ARCH))/board.mk
