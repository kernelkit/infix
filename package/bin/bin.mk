################################################################################
#
# bin
#
################################################################################

BIN_VERSION = 1.0
BIN_SITE_METHOD = local
BIN_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/bin
BIN_LICENSE = BSD-3-Clause
BIN_LICENSE_FILES = LICENSE
BIN_REDISTRIBUTE = NO
BIN_DEPENDENCIES = sysrepo libite \
	host-python3 python3 host-python-pypa-build host-python-installer \
	host-python-poetry-core
BIN_CONF_OPTS = --disable-silent-rules
BIN_AUTORECONF = YES

define BIN_CONF_ENV
CFLAGS="$(INFIX_CFLAGS)"
endef

define BIN_PERMISSIONS
	/usr/bin/copy  f 04750 root klish - - - - -
endef

define BIN_BUILD_PYTHON
	cd $(BIN_SITE) && \
		$(PKG_PYTHON_PEP517_ENV) $(HOST_DIR)/bin/python3 $(PKG_PYTHON_PEP517_BUILD_CMD) -o $(@D)/dist
	mkdir -p $(TARGET_DIR)/usr/bin
	rm -f $(TARGET_DIR)/usr/bin/show
	cd $(@D) && \
		$(HOST_DIR)/bin/python3 $(TOPDIR)/support/scripts/pyinstaller.py \
			dist/*.whl \
			--interpreter=/usr/bin/python3 \
			--script-kind=posix \
			--purelib=$(TARGET_DIR)/usr/lib/python$(PYTHON3_VERSION_MAJOR)/site-packages \
			--headers=$(TARGET_DIR)/usr/include/python$(PYTHON3_VERSION_MAJOR) \
			--scripts=$(TARGET_DIR)/usr/bin \
			--data=$(TARGET_DIR)
endef
BIN_POST_INSTALL_TARGET_HOOKS += BIN_BUILD_PYTHON

define BIN_INSTALL_BASH_COMPLETION
	install -D $(@D)/bash_completion.d/show \
		$(TARGET_DIR)/etc/bash_completion.d/show
endef
BIN_POST_INSTALL_TARGET_HOOKS += BIN_INSTALL_BASH_COMPLETION

$(eval $(autotools-package))
