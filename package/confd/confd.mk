################################################################################
#
# confd
#
################################################################################

CONFD_VERSION = 1.8
CONFD_SITE_METHOD = local
CONFD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/confd
CONFD_LICENSE = BSD-3-Clause
CONFD_LICENSE_FILES = LICENSE
CONFD_REDISTRIBUTE = NO
CONFD_DEPENDENCIES = host-sysrepo sysrepo rousette netopeer2 jansson libite sysrepo libsrx libglib2
CONFD_AUTORECONF = YES
CONFD_CONF_OPTS += --disable-silent-rules --with-crypt=$(BR2_PACKAGE_CONFD_DEFAULT_CRYPT)
CONFD_SYSREPO_SHM_PREFIX = sr_buildroot$(subst /,_,$(CONFIG_DIR))_confd

define CONFD_CONF_ENV
	CFLAGS="$(INFIX_CFLAGS)"
endef

ifeq ($(BR2_PACKAGE_PODMAN),y)
CONFD_CONF_OPTS += --enable-containers
else
CONFD_CONF_OPTS += --disable-containers
endif

ifeq ($(BR2_PACKAGE_FEATURE_WIFI),y)
CONFD_CONF_OPTS += --enable-wifi
else
CONFD_CONF_OPTS += --disable-wifi
endif
define CONFD_INSTALL_EXTRA
	for fn in confd.conf resolvconf.conf; do \
		cp $(CONFD_PKGDIR)/$$fn  $(FINIT_D)/available/; \
		ln -sf ../available/$$fn $(FINIT_D)/enabled/$$fn; \
	done
	cp $(CONFD_PKGDIR)/tmpfiles.conf $(TARGET_DIR)/etc/tmpfiles.d/confd.conf
	mkdir -p $(TARGET_DIR)/etc/avahi/services
	cp $(CONFD_PKGDIR)/avahi.service $(TARGET_DIR)/etc/avahi/services/netconf.service
endef

NETOPEER2_SEARCHPATH=$(TARGET_DIR)/usr/share/yang/modules/netopeer2/
SYSREPO_SEARCHPATH=$(TARGET_DIR)/usr/share/yang/modules/sysrepo/
LIBNETCONF2_SEARCHPATH=$(TARGET_DIR)/usr/share/yang/modules/libnetconf2/
CONFD_SEARCHPATH=$(TARGET_DIR)/usr/share/yang/modules/confd/
TEST_MODE_SEARCHPATH=$(TARGET_DIR)/usr/share/yang/modules/test-mode/
ROUSETTE_SEARCHPATH=$(TARGET_DIR)/usr/share/yang/modules/rousette/
COMMON_SYSREPO_ENV = \
	SYSREPO_SHM_PREFIX=$(CONFD_SYSREPO_SHM_PREFIX) \
	SYSREPOCTL_EXECUTABLE="$(HOST_DIR)/bin/sysrepoctl" \
	SYSREPOCFG_EXECUTABLE="$(HOST_DIR)/bin/sysrepocfg" \
	SEARCH_PATH="$(NETOPEER2_SEARCHPATH) $(SYSREPO_SEARCHPATH) $(LIBNETCONF2_SEARCHPATH) $(TEST_MODE_SEARCHPATH) $(CONFD_SEARCHPATH) $(ROUSETTE_SEARCHPATH)"


define CONFD_INSTALL_YANG_MODULES
	$(COMMON_SYSREPO_ENV) \
	$(BR2_EXTERNAL_INFIX_PATH)/utils/srload $(@D)/yang/sysrepo.inc
	$(COMMON_SYSREPO_ENV) \
	$(BR2_EXTERNAL_INFIX_PATH)/utils/srload $(@D)/yang/libnetconf2.inc
	$(COMMON_SYSREPO_ENV) \
	$(BR2_EXTERNAL_INFIX_PATH)/utils/srload $(@D)/yang/netopeer2.inc
	$(COMMON_SYSREPO_ENV) \
	$(BR2_EXTERNAL_INFIX_PATH)/utils/srload $(@D)/yang/rousette.inc
	$(COMMON_SYSREPO_ENV) \
	$(BR2_EXTERNAL_INFIX_PATH)/utils/srload $(@D)/yang/test-mode.inc
	$(COMMON_SYSREPO_ENV) \
	$(BR2_EXTERNAL_INFIX_PATH)/utils/srload $(@D)/yang/confd.inc
endef

ifeq ($(BR2_PACKAGE_PODMAN),y)
define CONFD_INSTALL_YANG_MODULES_CONTAINERS
	$(COMMON_SYSREPO_ENV) \
	$(BR2_EXTERNAL_INFIX_PATH)/utils/srload $(@D)/yang/containers.inc
endef
endif
ifeq ($(BR2_PACKAGE_FEATURE_WIFI),y)
define CONFD_INSTALL_YANG_MODULES_WIFI
	$(COMMON_SYSREPO_ENV) \
	$(BR2_EXTERNAL_INFIX_PATH)/utils/srload $(@D)/yang/wifi.inc
endef
endif

# PER_PACKAGE_DIR
# Since the last package in the dependency chain that runs sysrepoctl is confd, we need to
# manually copy the *real* content here from host-sysrepo.
ifeq ($(BR2_PER_PACKAGE_DIRECTORIES),y)
define CONFD_INSTALL_IN_ROMFS
	cp -a $(PER_PACKAGE_DIR)/host-sysrepo/target/etc/sysrepo/* $(PER_PACKAGE_DIR)/confd/target/etc/sysrepo/
endef
endif

# PER_PACKAGE_DIR
# Need to do some special stuff if using per-packet (parallel) since sysrepo install the submodules in
# $(PER_PACKAGE_DIR)/host-sysrepo/target/etc/sysrepo/ but $(PER_PACKAGE_DIR)/confd/target/etc/sysrepo/ contains remains
# of other packets that have installed its models (netopeer2), we want the result in $(PER_PACKAGE_DIR)/host-sysrepo/target/etc/sysrepo/
define CONFD_EMPTY_SYSREPO
	rm -rf $(TARGET_DIR)/etc/sysrepo/*
	if [ "$(BR2_PER_PACKAGE_DIRECTORIES)" = "y" ]; then \
		rm -rf $(PER_PACKAGE_DIR)/host-sysrepo/target/etc/sysrepo/* $(PER_PACKAGE_DIR)/confd/target/etc/sysrepo/*; \
	fi
endef
define CONFD_CLEANUP
	rm -f /dev/shm/$(CONFD_SYSREPO_SHM_PREFIX)*
endef
CONFD_PRE_BUILD_HOOKS += CONFD_EMPTY_SYSREPO
CONFD_PRE_BUILD_HOOKS += CONFD_CLEANUP
CONFD_POST_INSTALL_TARGET_HOOKS += CONFD_INSTALL_EXTRA
CONFD_POST_INSTALL_TARGET_HOOKS += CONFD_INSTALL_YANG_MODULES
CONFD_POST_INSTALL_TARGET_HOOKS += CONFD_INSTALL_YANG_MODULES_CONTAINERS
CONFD_POST_INSTALL_TARGET_HOOKS += CONFD_INSTALL_YANG_MODULES_WIFI
CONFD_POST_INSTALL_TARGET_HOOKS += CONFD_INSTALL_IN_ROMFS
CONFD_TARGET_FINALIZE_HOOKS += CONFD_CLEANUP

$(eval $(autotools-package))
