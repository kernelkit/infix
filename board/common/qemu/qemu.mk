################################################################################
#
# qemu-scripts
#
################################################################################

qemu-scripts-dir := $(pkgdir)/$(if $(IMAGE_DDI),ddi,itb)
qemu-image := ../$(INFIX_ARTIFACT).$(if $(IMAGE_DDI),raw,qcow2)
qemu-disk := $(if $(IMAGE_DDI_DISK),../$(INFIX_ARTIFACT).disk)
qemu-esp := $(if $(IMAGE_BAREBOX_ESP),../barebox-esp.vfat)

qemu-kconfig-prefix := $(if $(IMAGE_DDI),,CONFIG_)

qemu-kconfig = \
	CONFIG_="$(qemu-kconfig-prefix)" \
	BR2_CONFIG="$(BINARIES_DIR)/qemu/.config" \
	$(BUILD_DIR)/buildroot-config/$(1) $(2) "$(BINARIES_DIR)/qemu/Config.in"

ifeq ($(QEMU_SCRIPTS),y)

.PHONY: run
run:
	@$(BINARIES_DIR)/qemu/run.sh

.PHONY: run-menuconfig
run-menuconfig: $(BUILD_DIR)/buildroot-config/mconf
	@$(call qemu-kconfig,mconf)

qemu-scripts: \
		$(BINARIES_DIR)/qemu/run.sh \
		$(BINARIES_DIR)/qemu/Config.in \
		$(BINARIES_DIR)/qemu/.config

$(BINARIES_DIR)/qemu/run.sh: $(qemu-scripts-dir)/run.sh
	@$(call IXMSG,"Installing QEMU scripts")
	@mkdir -p $(dir $@)
	@cp $< $@

$(BINARIES_DIR)/qemu/Config.in: $(qemu-scripts-dir)/Config.in.in
	@mkdir -p $(dir $@)
	@sed \
		-e "s:@ARCH@:$(BR2_ARCH):g" \
		-e "s:@IMAGE@:$(qemu-image):g" \
		-e "s:@DISK@:$(qemu-disk):g" \
		-e "s:@ESP@:$(qemu-esp):g" \
	<$< >$@

$(BINARIES_DIR)/qemu/.config: $(BINARIES_DIR)/qemu/Config.in
	@$(call qemu-kconfig,conf,--olddefconfig)
	@rm -f $@.old

TARGETS_ROOTFS += qemu-scripts
endif
