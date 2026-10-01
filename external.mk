include $(BR2_EXTERNAL_INFIX_PATH)/infix.mk
include $(sort $(wildcard $(BR2_EXTERNAL_INFIX_PATH)/package/*/*.mk))
include $(BR2_EXTERNAL_INFIX_PATH)/board/board.mk
include $(BR2_EXTERNAL_INFIX_PATH)/test/test.mk

.PHONY: local.mk
local.mk:
	@$(call IXMSG,"Installing local override for certain packages")
	@(cd $O && ln -s $(BR2_EXTERNAL_INFIX_PATH)/local.mk .)

#
# Buildroot package extensions
#
define FRR_POST_BUILD_HOOK
	mkdir -p $(TARGET_DIR)/etc/iproute2/
	cp -r $(@D)/tools/etc/iproute2/rt_protos.d/ $(TARGET_DIR)/etc/iproute2/
endef

FRR_POST_BUILD_HOOKS += FRR_POST_BUILD_HOOK

#
# The SNMP agent is read-only, see doc/snmp.md.  Drop SET support from
# the build rather than leave it to the generated VACM configuration to
# withhold, so a mistake there cannot become a writable agent.
#
NETSNMP_CONF_OPTS += --enable-read-only

# Some of these assumes the presence of systemd, so skip them.
LVM2_CONF_OPTS += --disable-udev_rules

#
# The multipath-tools package, which we need for kpartx, installs udev
# rules that assumes the presence of systemd. There is no option to
# skip the install, but we can pick the destination. So place them in
# the root and then remove them in the post-install hook.
MULTIPATH_TOOLS_OPTS += udevrulesdir="/.mpath-trash"

define MPATH_POST_INSTALL_CLEANUP
	rm -rf $(TARGET_DIR)/.mpath-trash
endef

MULTIPATH_TOOLS_POST_INSTALL_TARGET_HOOKS += MPATH_POST_INSTALL_CLEANUP

#
# External pre-built toolchains do not carry their own license.
#
# The Bootlin toolchains used by Infix are built from Buildroot and
# compose a .csv file of all components included in the toolchain.
#
define TOOLCHAIN_BOOTLIN_POST_HOOK
	cp $(TOOLCHAIN_EXTERNAL_DOWNLOAD_INSTALL_DIR)/summary.csv \
		$(LEGAL_INFO_DIR)/toolchain-external-bootlin.csv
endef

TOOLCHAIN_EXTERNAL_BOOTLIN_POST_LEGAL_INFO_HOOKS += TOOLCHAIN_BOOTLIN_POST_HOOK
