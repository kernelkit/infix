ifeq ($(IX_IMAGE_DDI),y)

IX_INITRAMFS_DIR   := $(BR2_EXTERNAL_INFIX_PATH)/board/common/initramfs
IX_INITRAMFS_BUILD := $(BUILD_DIR)/infix-initramfs

define LINUX_PRE_BUILD_INITRAMFS
	@$(call IXMSG,"Creating Infix initramfs")
	mkdir -p $(IX_INITRAMFS_BUILD)
	cp -a $(IX_INITRAMFS_DIR)/modules.d/* $(HOST_DIR)/lib/dracut/modules.d/
	$(HOST_DIR)/bin/fakeroot $(HOST_DIR)/bin/dracut \
		-c $(IX_INITRAMFS_DIR)/dracut.conf \
	 	--sysroot $(TARGET_DIR) \
	 	--tmpdir $(IX_INITRAMFS_BUILD) \
	 	-M \
	 	--force \
	 	--no-compress \
	 	$(@D)/infix-initramfs.cpio
endef
LINUX_PRE_BUILD_HOOKS += LINUX_PRE_BUILD_INITRAMFS
endif
