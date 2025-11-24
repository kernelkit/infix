include $(BR2_EXTERNAL_INFIX_PATH)/board/common/image/ix-image.mk
include $(sort $(wildcard $(BR2_EXTERNAL_INFIX_PATH)/board/common/image/*/*.mk))
