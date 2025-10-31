include $(BR2_EXTERNAL_INFIX_PATH)/board/ix-board.mk
include $(sort $(wildcard $(BR2_EXTERNAL_INFIX_PATH)/board/*/*/*.mk))
