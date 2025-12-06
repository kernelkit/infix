################################################################################
#
# statd
#
################################################################################

STATD_VERSION = 1.0
STATD_SITE_METHOD = local
STATD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/statd
STATD_LICENSE = BSD-3-Clause
STATD_LICENSE_FILES = LICENSE
STATD_REDISTRIBUTE = NO
STATD_DEPENDENCIES = sysrepo libev libsrx jansson libyang libite \
	host-python3 python3 host-python-pypa-build host-python-installer \
	host-python-poetry-core dbus-python
STATD_AUTORECONF = YES

define STATD_CONF_ENV
CFLAGS="$(INFIX_CFLAGS)"
endef

STATD_CONF_OPTS = --prefix= --disable-silent-rules

ifeq ($(BR2_PACKAGE_PODMAN),y)
STATD_CONF_OPTS += --enable-containers
else
STATD_CONF_OPTS += --disable-containers
endif

define STATD_BUILD_PYTHON
	cd $(STATD_SITE)/python && \
		$(PKG_PYTHON_PEP517_ENV) $(HOST_DIR)/bin/python3 $(PKG_PYTHON_PEP517_BUILD_CMD) -o $(@D)/python/dist
	mkdir -p $(TARGET_DIR)/usr/libexec/statd
	cd $(@D)/python && \
		$(HOST_DIR)/bin/python3 $(TOPDIR)/support/scripts/pyinstaller.py \
			dist/*.whl \
			--interpreter=/usr/bin/python3 \
			--script-kind=posix \
			--purelib=$(TARGET_DIR)/usr/lib/python$(PYTHON3_VERSION_MAJOR)/site-packages \
			--headers=$(TARGET_DIR)/usr/include/python$(PYTHON3_VERSION_MAJOR) \
			--scripts=$(TARGET_DIR)/usr/libexec/statd \
			--data=$(TARGET_DIR)
endef
STATD_POST_INSTALL_TARGET_HOOKS += STATD_BUILD_PYTHON

define STATD_INSTALL_EXTRA
	cp $(STATD_PKGDIR)/statd.conf  $(FINIT_D)/available/
	ln -sf ../available/statd.conf $(FINIT_D)/enabled/statd.conf
endef
STATD_TARGET_FINALIZE_HOOKS += STATD_INSTALL_EXTRA

$(eval $(autotools-package))
