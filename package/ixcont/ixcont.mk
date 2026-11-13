################################################################################
#
# ixcont
#
################################################################################

IXCONT_VERSION = 1.0
IXCONT_SITE_METHOD = local
IXCONT_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/ixcont
IXCONT_LICENSE = BSD-3-Clause
IXCONT_LICENSE_FILES = LICENSE
IXCONT_REDISTRIBUTE = NO
IXCONT_DEPENDENCIES = crun python3 \
	host-python3 host-python-pypa-build host-python-installer \
	host-python-poetry-core

define IXCONT_BUILD_CMDS
	cd $(IXCONT_SITE) && \
		$(PKG_PYTHON_PEP517_ENV) $(HOST_DIR)/bin/python3 $(PKG_PYTHON_PEP517_BUILD_CMD) -o $(@D)/dist
endef

define IXCONT_INSTALL_TARGET_CMDS
	mkdir -p $(TARGET_DIR)/usr/libexec/ixcont
	rm -f $(TARGET_DIR)/usr/libexec/ixcont/ixcont
	cd $(@D) && \
		$(HOST_DIR)/bin/python3 $(TOPDIR)/support/scripts/pyinstaller.py \
			dist/*.whl \
			--interpreter=/usr/bin/python3 \
			--script-kind=posix \
			--purelib=$(TARGET_DIR)/usr/lib/python$(PYTHON3_VERSION_MAJOR)/site-packages \
			--headers=$(TARGET_DIR)/usr/include/python$(PYTHON3_VERSION_MAJOR) \
			--scripts=$(TARGET_DIR)/usr/libexec/ixcont \
			--data=$(TARGET_DIR)
	ln -sf /usr/libexec/ixcont/ixcont $(TARGET_DIR)/usr/sbin/ixcont
	$(INSTALL) -D -m 0644 $(IXCONT_PKGDIR)/container@.conf $(FINIT_D)/available/container@.conf
	$(INSTALL) -D -m 0644 $(IXCONT_PKGDIR)/ixcont.conf $(FINIT_D)/available/ixcont.conf
	mkdir -p $(FINIT_D)/enabled
	ln -sf ../available/ixcont.conf $(FINIT_D)/enabled/ixcont.conf
	$(INSTALL) -D -m 0644 $(IXCONT_PKGDIR)/policy.json $(TARGET_DIR)/etc/containers/policy.json
	$(INSTALL) -D -m 0644 $(IXCONT_PKGDIR)/registries.conf $(TARGET_DIR)/etc/containers/registries.conf
	mkdir -p $(TARGET_DIR)/lib/oci
endef

$(eval $(generic-package))

# Flatten an OCI archive into $(TARGET_DIR)/lib/oci/<name>, for packages
# that embed container images in the rootfs.  Usage from a package .mk:
#
#   $(call IXCONT_FLATTEN,$(FOO_DL_DIR)/$(FOO_SOURCE),foo-$(FOO_VERSION))
#
IXCONT_FLATTEN = $(HOST_DIR)/bin/python3 -I $(IXCONT_SITE)/ixcont/oci.py flatten \
	--arch $(GO_GOARCH) $(1) $(TARGET_DIR)/lib/oci/$(2)
