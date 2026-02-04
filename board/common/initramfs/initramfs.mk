ifeq ($(IX_IMAGE_DDI),y)

IX_INITRAMFS_DIR   := $(BR2_EXTERNAL_INFIX_PATH)/board/common/initramfs
IX_INITRAMFS_BUILD := $(BUILD_DIR)/infix-initramfs
IX_INITRAMFS_DEPS  := \
	host-dracut \
	host-fakeroot \
	busybox \
	cryptsetup \
	gptfdisk \
	jq \
	lvm2 \
	multipath-tools \
	util-linux

define LINUX_PRE_BUILD_INITRAMFS
	@$(call IXMSG,"Creating Infix initramfs")
	mkdir -p $(IX_INITRAMFS_BUILD)
	cp -a $(IX_INITRAMFS_DIR)/modules.d/* $(HOST_DIR)/lib/dracut/modules.d/
	$(HOST_DIR)/bin/fakeroot $(HOST_DIR)/bin/dracut \
		-c $(IX_INITRAMFS_DIR)/dracut.conf \
		--kver $(LINUX_VERSION) --no-kernel \
	 	--sysroot $(TARGET_DIR) \
	 	--tmpdir $(IX_INITRAMFS_BUILD) \
	 	-M \
	 	--force \
	 	--no-compress \
	 	$(@D)/infix-initramfs.cpio
endef
LINUX_PRE_BUILD_HOOKS += LINUX_PRE_BUILD_INITRAMFS

# We are late to the party here, buildroot/linux/linux.mk has already
# been sourced, so in addition to the dependency list, we also need to
# "manually" extend the deps for the kernel's configure target.
LINUX_DEPENDENCIES += $(IX_INITRAMFS_DEPS)
$(LINUX_TARGET_CONFIGURE): | $(IX_INITRAMFS_DEPS)

endif
