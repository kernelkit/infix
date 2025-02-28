################################################################################
#
# confd
#
################################################################################

CONFD_VERSION = 1.5
CONFD_SITE_METHOD = local
CONFD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/confd
CONFD_LICENSE = BSD-3-Clause
CONFD_LICENSE_FILES = LICENSE
CONFD_REDISTRIBUTE = NO
CONFD_DEPENDENCIES = host-sysrepo sysrepo netopeer2 jansson libite sysrepo libsrx libglib2
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

define CONFD_INSTALL_EXTRA
	for fn in confd.conf resolvconf.conf; do \
		cp $(CONFD_PKGDIR)/$$fn  $(FINIT_D)/available/; \
		ln -sf ../available/$$fn $(FINIT_D)/enabled/$$fn; \
	done
	cp $(CONFD_PKGDIR)/tmpfiles.conf $(TARGET_DIR)/etc/tmpfiles.d/confd.conf
	mkdir -p $(TARGET_DIR)/etc/avahi/services
	cp $(CONFD_PKGDIR)/avahi.service $(TARGET_DIR)/etc/avahi/services/netconf.service
endef

COMMON_SYSREPO_ENV = \
	SYSREPO_SHM_PREFIX=$(CONFD_SYSREPO_SHM_PREFIX) \
	SYSREPOCTL_EXECUTABLE="$(HOST_DIR)/bin/sysrepoctl" \
	SYSREPOCFG_EXECUTABLE="$(HOST_DIR)/bin/sysrepocfg" \
	SEARCH_PATH="$(TARGET_DIR)/usr/share/yang/modules/confd/"

define CONFD_INSTALL_YANG_MODULES
	$(COMMON_SYSREPO_ENV) \
	$(BR2_EXTERNAL_INFIX_PATH)/utils/srload $(@D)/yang/confd.inc
endef

ifeq ($(BR2_PACKAGE_PODMAN),y)
define CONFD_INSTALL_YANG_MODULES_CONTAINERS
	$(COMMON_SYSREPO_ENV) \
	$(BR2_EXTERNAL_INFIX_PATH)/utils/srload $(@D)/yang/containers.inc
endef
endif

define CONFD_PERMISSIONS
	/etc/sysrepo/data/ r 660 root wheel - - - - -
	/etc/sysrepo/data  d 770 root wheel - - - - -
endef

define CONFD_CLEANUP
	rm -f /dev/shm/$(CONFD_SYSREPO_SHM_PREFIX)*
endef

CONFD_PRE_INSTALL_TARGET_HOOKS += CONFD_CLEANUP
CONFD_POST_INSTALL_TARGET_HOOKS += CONFD_INSTALL_EXTRA
CONFD_POST_INSTALL_TARGET_HOOKS += CONFD_INSTALL_YANG_MODULES
CONFD_POST_INSTALL_TARGET_HOOKS += CONFD_INSTALL_YANG_MODULES_CONTAINERS
CONFD_POST_INSTALL_TARGET_HOOKS += CONFD_CLEANUP

$(eval $(autotools-package))
