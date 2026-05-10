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
MODEMD_DEPENDENCIES = modem-manager jansson python3 \
	host-python3 host-python-pypa-build host-python-installer \
	host-python-poetry-core

define MODEMD_BUILD_CMDS
    $(TARGET_CC) $(TARGET_CFLAGS) $(TARGET_LDFLAGS) \
        $(MODEMD_DIR)/modem-command.c -o $(MODEMD_DIR)/modem-command -ljansson
endef

define MODEMD_BUILD_PYTHON
	cd $(MODEMD_SITE) && \
		$(PKG_PYTHON_PEP517_ENV) $(HOST_DIR)/bin/python3 $(PKG_PYTHON_PEP517_BUILD_CMD) -o $(@D)/dist
	mkdir -p $(TARGET_DIR)/usr/libexec/modemd
	rm -f $(TARGET_DIR)/usr/libexec/modemd/modemd \
		$(TARGET_DIR)/usr/libexec/modemd/modem-* \
		$(TARGET_DIR)/usr/libexec/modemd/sim-setup
	cd $(@D) && \
		$(HOST_DIR)/bin/python3 $(TOPDIR)/support/scripts/pyinstaller.py \
			dist/*.whl \
			--interpreter=/usr/bin/python3 \
			--script-kind=posix \
			--purelib=$(TARGET_DIR)/usr/lib/python$(PYTHON3_VERSION_MAJOR)/site-packages \
			--headers=$(TARGET_DIR)/usr/include/python$(PYTHON3_VERSION_MAJOR) \
			--scripts=$(TARGET_DIR)/usr/libexec/modemd \
			--data=$(TARGET_DIR)
endef
MODEMD_POST_INSTALL_TARGET_HOOKS += MODEMD_BUILD_PYTHON

define MODEMD_INSTALL_TARGET_CMDS
	mkdir -p $(TARGET_DIR)/usr/libexec/modemd
	mkdir -p $(TARGET_DIR)/lib/udev/rules.d
	mkdir -p $(FINIT_D)/available/
	mkdir -p $(TARGET_DIR)/sbin
	$(INSTALL) -D -m 0644 $(MODEMD_DIR)/finit.conf $(FINIT_D)/available/modemd.conf
	install -m 644 $(MODEMD_DIR)/modemd.rules $(TARGET_DIR)/lib/udev/rules.d/90-modemd.rules
	install -m 644 $(MODEMD_DIR)/qmi-wwan-ids.rules $(TARGET_DIR)/lib/udev/rules.d/91-qmi-wwan-ids.rules
	install -m 644 $(MODEMD_DIR)/77-mm-dell-port-types.rules $(TARGET_DIR)/etc/udev/rules.d/77-mm-dell-port-types.rules
	install -m 644 $(MODEMD_DIR)/77-mm-modem-gps.rules $(TARGET_DIR)/etc/udev/rules.d/77-mm-modem-gps.rules
	install -D -m 644 $(MODEMD_DIR)/modemd.modules-load $(TARGET_DIR)/etc/modules-load.d/modemd.conf
	install -m 755 $(MODEMD_DIR)/modem-command $(TARGET_DIR)/sbin/modem-command
	ln -sf /usr/libexec/modemd/modemd $(TARGET_DIR)/sbin/modemd
endef

$(eval $(generic-package))
