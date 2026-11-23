ifeq ($(BR2_PACKAGE_SAILFISH_SALINE)$(BR2_PACKAGE_MODULES_NET_DSA_PASW_TOOL),yy)
define INSTALL_SALINE_REGS
	$(INSTALL) -D -m 0755 $(@D)/drivers/net/dsa/pasw/dev-saline/saline-regs.yml \
		$(TARGET_DIR)/etc/pasw/saline-regs.yml
endef
MODULES_POST_INSTALL_TARGET_HOOKS += INSTALL_SALINE_REGS
endif

$(eval $(ix-board))
$(eval $(generic-package))

sailfish-saline-install-udev:
	cp $(SAILFISH_SALINE_SITE)/99-sailfish-saline-vfio.rules /etc/udev/rules.d/
	udevadm trigger -c add -s pci -a vendor=0x10ee

sailfish-saline-check:
	$(SAILFISH_SALINE_SITE)/setup.sh check $(SALINE)

ifeq ($(BR2_PACKAGE_SAILFISH_SALINE),y)
sailfish-saline-run:
	$(SAILFISH_SALINE_SITE)/setup.sh \
		configure $(SALINE) $(BINARIES_DIR)/qemu/.config \
	&& $(MAKE) -C $(O) run
endif
