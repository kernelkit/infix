################################################################################
#
# qemu-scripts
#
################################################################################

QEMU_SCRIPTS_DIR := $(pkgdir)
qemu-kconfig = \
	CONFIG_="CONFIG_" \
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

$(BINARIES_DIR)/qemu/run.sh: $(QEMU_SCRIPTS_DIR)/run.sh
	@$(call IXMSG,"Installing QEMU scripts")
	@mkdir -p $(dir $@)
	@cp $< $@

$(BINARIES_DIR)/qemu/Config.in: $(QEMU_SCRIPTS_DIR)/Config.in.in
	@mkdir -p $(dir $@)
	@sed \
		-e "s:@ARCH@:QEMU_$(BR2_ARCH):" \
		-e "s:@DISK_IMG@:../$(INFIX_ARTIFACT).qcow2:" \
	< $< >$@

$(BINARIES_DIR)/qemu/.config: $(BINARIES_DIR)/qemu/Config.in
	@$(call qemu-kconfig,conf,--olddefconfig)
	@rm -f $@.old

TARGETS_ROOTFS += qemu-scripts
endif
