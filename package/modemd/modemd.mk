################################################################################
#
# modemd
#
################################################################################

MODEMD_VERSION = 1.0
MODEMD_SITE_METHOD = local
MODEMD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/modemd
MODEMD_LICENSE = BSD-3-Clause
MODEMD_LICENSE_FILES = LICENSE
MODEMD_REDISTRIBUTE = NO
MODEMD_DEPENDENCIES = modem-manager jansson python3

define MODEMD_BUILD_CMDS
    $(TARGET_CC) $(TARGET_CFLAGS) $(TARGET_LDFLAGS) \
        $(MODEMD_DIR)/modem-command.c -o $(MODEMD_DIR)/modem-command -ljansson
endef

define MODEMD_INSTALL_TARGET_CMDS
	mkdir -p $(TARGET_DIR)/usr/libexec/modemd
	mkdir -p $(TARGET_DIR)/lib/udev/rules.d
	mkdir -p $(FINIT_D)/available/
	mkdir -p $(TARGET_DIR)/sbin
	$(INSTALL) -D -m 0644 $(MODEMD_DIR)/finit.conf $(FINIT_D)/available/modemd.conf
	install -m 644 $(MODEMD_DIR)/modemd.rules $(TARGET_DIR)/lib/udev/rules.d/90-modemd.rules
	install -m 644 $(MODEMD_DIR)/qmi-wwan-ids.rules $(TARGET_DIR)/lib/udev/rules.d/91-qmi-wwan-ids.rules
	install -m 644 $(MODEMD_DIR)/77-mm-dell-port-types.rules $(TARGET_DIR)/etc/udev/rules.d/77-mm-dell-port-types.rules
	install -D -m 644 $(MODEMD_DIR)/modemd.modules-load $(TARGET_DIR)/etc/modules-load.d/modemd.conf
	install -m 755 $(MODEMD_DIR)/modem-udev $(TARGET_DIR)/usr/libexec/modemd/
	install -m 755 $(MODEMD_DIR)/modemd $(TARGET_DIR)/sbin/modemd
	install -m 755 $(MODEMD_DIR)/modem-command $(TARGET_DIR)/sbin/modem-command
	install -m 755 $(MODEMD_DIR)/modem-info $(TARGET_DIR)/usr/libexec/modemd/
	install -m 755 $(MODEMD_DIR)/modem-rpc $(TARGET_DIR)/usr/libexec/modemd/
	install -m 755 $(MODEMD_DIR)/modem-sms $(TARGET_DIR)/usr/libexec/modemd/
	install -m 755 $(MODEMD_DIR)/modem-scan-networks $(TARGET_DIR)/usr/libexec/modemd/
	install -m 755 $(MODEMD_DIR)/modem-update-firmware $(TARGET_DIR)/usr/libexec/modemd/
	install -m 755 $(MODEMD_DIR)/modem-ussd $(TARGET_DIR)/usr/libexec/modemd/
	install -m 755 $(MODEMD_DIR)/modem-carrier $(TARGET_DIR)/usr/libexec/modemd/
	install -m 755 $(MODEMD_DIR)/modem-power $(TARGET_DIR)/usr/libexec/modemd/
	install -m 755 $(MODEMD_DIR)/sim-setup $(TARGET_DIR)/usr/libexec/modemd/
endef

$(eval $(generic-package))
