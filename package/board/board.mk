include $(BR2_EXTERNAL_INFIX_PATH)/package/board/ix-board.mk
include $(sort $(wildcard $(BR2_EXTERNAL_INFIX_PATH)/package/board/*/*.mk))
