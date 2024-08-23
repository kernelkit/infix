PYTHON_STATD_VERSION = 1.0
PYTHON_STATD_SITE_METHOD = local
PYTHON_STATD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/statd/python
PYTHON_STATD_LICENSE = BSD-3-Clause
PYTHON_STATD_LICENSE_FILES = LICENSE
PYTHON_STATD_DEPENDENCIES = host-python3 python3 host-python-poetry-core dbus-python
PYTHON_STATD_SETUP_TYPE = pep517 # poetry

define PYTHON_STATD_MOVE_BINARIES
	mkdir -p $(TARGET_DIR)/usr/libexec/statd
	mv $(TARGET_DIR)/usr/bin/yanger $(TARGET_DIR)/usr/libexec/statd/
	mv $(TARGET_DIR)/usr/bin/cli-pretty $(TARGET_DIR)/usr/libexec/statd/
	mv $(TARGET_DIR)/usr/bin/ospf-status $(TARGET_DIR)/usr/libexec/statd/
	mv $(TARGET_DIR)/usr/bin/dhcp-server-status $(TARGET_DIR)/usr/libexec/statd/
endef
PYTHON_STATD_POST_INSTALL_TARGET_HOOKS += PYTHON_STATD_MOVE_BINARIES
 $(eval $(python-package))
