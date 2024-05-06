################################################################################
#
# confd
#
################################################################################


CONFD_VERSION = 1.0
CONFD_SITE_METHOD = local
CONFD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/confd
CONFD_LICENSE = BSD-3-Clause
CONFD_LICENSE_FILES = LICENSE
CONFD_REDISTRIBUTE = NO
CONFD_DEPENDENCIES = host-sysrepo netopeer2 augeas jansson libite sysrepo libsrx libglib2
CONFD_AUTORECONF = YES

CONFD_SYSREPO_SHM_PREFIX = sr_buildroot$(subst /,_,$(CONFIG_DIR))_netopeer2

echo "prefix: $(CONFD_SYSREPO_SHM_PREFIX)"
define CONFD_CONF_ENV
CFLAGS="$(INFIX_CFLAGS)"
endef

ifeq ($(BR2_PACKAGE_PODMAN),y)
CONFD_CONF_OPTS += --enable-containers
else
CONFD_CONF_OPTS += --disable-containers
endif


CONFD_MAKE_ENV = \
	SYSREPOCTL_EXECUTABLE=$(HOST_DIR)/bin/sysrepoctl \
	SYSREPO_SHM_PREFIX=$(CONFD_SYSREPO_SHM_PREFIX)

define CONFD_INSTALL_EXTRA
	cp $(CONFD_PKGDIR)/confd.conf  $(FINIT_D)/available/
	ln -sf ../available/confd.conf $(FINIT_D)/enabled/confd.conf
	cp $(CONFD_PKGDIR)/tmpfiles.conf $(TARGET_DIR)/etc/tmpfiles.d/confd.conf
	mkdir -p $(TARGET_DIR)/etc/avahi/services
	cp $(CONFD_PKGDIR)/avahi.service $(TARGET_DIR)/etc/avahi/services/netconf.service
endef
define CONFD_INSTALL_YANG_MODULES
	USE_CONTAINERS=$(BR2_PACKAGE_PODMAN) SYSREPO_SHM_PREFIX=$(CONFD_SYSREPO_SHM_PREFIX) SYSREPOCTL_EXECUTABLE="$(HOST_DIR)/bin/sysrepoctl" SYSREPOCFG_EXECUTABLE="$(HOST_DIR)/bin/sysrepocfg" SEARCH_PATH="$(TARGET_DIR)/usr/share/yang/modules/confd/" $(@D)/scripts/setup.sh
endef
define CONFD_PERMISSIONS
# generated using chatGPT
/etc/sysrepo/data/iana-ssh-mac-algs.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-nmda.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-with-defaults.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-acm.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/libnetconf2-netconf-server.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-acm.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-factory-default.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-factory-default.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-tls-cipher-suite-algs.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-client.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-keystore.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-dot1q-types.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-ethernet-interface.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-monitoring.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tls-common.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tls-server.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-system.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-nmda.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-server.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-system.startup f 660 root wheel - - - - -
/etc/sysrepo/data/infix-factory-default.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-key-exchange-algs.startup f 660 root wheel - - - - -
/etc/sysrepo/data/libnetconf2-netconf-server.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-server.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-routing.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-datastores.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-interfaces.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-network-instance.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-server.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-client.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-routing.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-routing.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-schema-mount.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-crypto-types.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ip.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-crypto-types.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-services.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-dot1ab-lldp.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-lldp.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-schema-mount.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-factory-default.startup f 660 root wheel - - - - -
/etc/sysrepo/data/notifications.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-ip.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-plugind.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-hardware.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-subscribed-notifications.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-key-chain.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ipv6-unicast-routing.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-keystore.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-monitoring.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-lldp.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-interfaces.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-system.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-schema-mount.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-dot1q-types.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-if-type.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-ethernet-interface.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-library.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-with-defaults.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-encryption-algs.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-interfaces.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-routing.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-truststore.startup f 660 root wheel - - - - -
/etc/sysrepo/data/iana-hardware.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-crypt-hash.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-dot1q-types.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-hardware.startup f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-plugind.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ssh-common.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-subscribed-notifications.startup f 660 root wheel - - - - -
/etc/sysrepo/data/iana-if-type.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-if-type.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ipv4-unicast-routing.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-with-defaults.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-dot1q-types.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-library.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-common.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-subscribed-notifications.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-interfaces.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ipv4-unicast-routing.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-monitoring.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-public-key-algs.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-plugind.startup f 660 root wheel - - - - -
/etc/sysrepo/data/infix-containers.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-hardware.startup f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-key-exchange-algs.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-dot1ab-lldp.startup f 660 root wheel - - - - -
/etc/sysrepo/data/infix-ethernet-interface.startup f 660 root wheel - - - - -
/etc/sysrepo/data/notifications.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-routing.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-ethernet-interface.startup f 660 root wheel - - - - -
/etc/sysrepo/data/iana-timezones.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-datastores.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-x509-cert-to-name.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-factory-default.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-dot1ab-lldp.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-acm.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-factory-default.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-containers.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/iana-tls-cipher-suite-algs.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-push.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-common.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-factory-default.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-if-type.startup f 660 root wheel - - - - -
/etc/sysrepo/data/nc-notifications.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tls-server.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/yang.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ip.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-hardware.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-push.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/notifications.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-notifications.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-public-key-algs.startup f 660 root wheel - - - - -
/etc/sysrepo/data/nc-notifications.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-interfaces.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-factory-default.startup f 660 root wheel - - - - -
/etc/sysrepo/data/iana-tls-cipher-suite-algs.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-hardware.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-encryption-algs.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-crypt-hash.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-network-instance.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-crypto-types.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-datastores.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-hardware.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-monitoring.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-datastores.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-hardware.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-ethernet-interface.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/yang.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-notifications.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ssh-common.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tls-common.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-ethernet-interface.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-mac-algs.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-server.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-services.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-x509-cert-to-name.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/iana-timezones.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-datastores.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/libnetconf2-netconf-server.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tls-common.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-library.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-notifications.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-server.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-mac-algs.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-monitoring.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-server.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-crypto-types.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-public-key-algs.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-shell-type.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-routing.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-nmda.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-monitoring.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-truststore.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-shell-type.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-ethernet-interface.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/iana-tls-cipher-suite-algs.startup f 660 root wheel - - - - -
/etc/sysrepo/data/infix-interfaces.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tls-server.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ipv6-unicast-routing.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-lldp.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-network-instance.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-ip.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/nc-notifications.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-client.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-routing.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ssh-common.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-keystore.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-dot1ab-lldp.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-notifications.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-client.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-origin.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-system.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-library.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-factory-default.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-timezones.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-subscribed-notifications.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-key-chain.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ipv4-unicast-routing.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/iana-crypt-hash.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-encryption-algs.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-notifications.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/iana-hardware.startup f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-encryption-algs.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ospf.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-key-exchange-algs.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-nmda.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-timezones.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ip.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-containers.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-ethernet-interface.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-interfaces.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-origin.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/libnetconf2-netconf-server.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-push.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-nmda.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ssh-common.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-monitoring.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ip.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-acm.startup f 660 root wheel - - - - -
/etc/sysrepo/data/infix-if-type.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tls-common.startup f 660 root wheel - - - - -
/etc/sysrepo/data/infix-dhcp-client.startup f 660 root wheel - - - - -
/etc/sysrepo/data/infix-hardware.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-system.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ssh-common.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-monitoring.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tls-server.startup f 660 root wheel - - - - -
/etc/sysrepo/data/infix-ip.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-with-defaults.startup f 660 root wheel - - - - -
/etc/sysrepo/data/infix-system.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ssh-server.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ipv4-unicast-routing.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-ethernet-interface.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/yang.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-hardware.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-x509-cert-to-name.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-common.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-if-type.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-server.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-origin.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-key-chain.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-common.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-system.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-x509-cert-to-name.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/nc-notifications.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-keystore.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/iana-hardware.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-common.startup f 660 root wheel - - - - -
/etc/sysrepo/data/infix-containers.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-server.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-interfaces.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ssh-server.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-key-chain.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-truststore.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-factory-default.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-if-type.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tls-server.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-keystore.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-routing.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ssh-server.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-plugind.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-client.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-shell-type.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-lldp.startup f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-mac-algs.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-interfaces.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-services.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-subscribed-notifications.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-if-type.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-factory-default.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-public-key-algs.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-if-type.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-mac-algs.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-factory-default.startup f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-monitoring.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tls-common.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-hardware.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-dhcp-client.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-system.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-if-type.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-truststore.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-schema-mount.startup f 660 root wheel - - - - -
/etc/sysrepo/data/infix-dhcp-client.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-network-instance.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-tcp-server.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ospf.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-truststore.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-crypt-hash.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-routing.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ipv6-unicast-routing.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/yang.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-routing.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/notifications.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-ip.startup f 660 root wheel - - - - -
/etc/sysrepo/data/libnetconf2-netconf-server.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-schema-mount.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-factory-default.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/yang.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-dot1ab-lldp.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-monitoring.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-key-chain.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-hardware.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ip.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-dhcp-client.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ipv6-unicast-routing.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-timezones.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-system.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-network-instance.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-hardware.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-ip.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ipv6-unicast-routing.startup f 660 root wheel - - - - -
/etc/sysrepo/data/infix-services.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-origin.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-library.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ospf.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-acm.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-push.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-containers.startup f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-encryption-algs.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ssh-server.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-ethernet-interface.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-services.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ieee802-dot1q-types.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-origin.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-public-key-algs.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ospf.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-shell-type.startup f 660 root wheel - - - - -
/etc/sysrepo/data/iana-crypt-hash.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-interfaces.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-yang-push.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/notifications.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/nc-notifications.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-server.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ospf.startup f 660 root wheel - - - - -
/etc/sysrepo/data/infix-lldp.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-x509-cert-to-name.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-factory-default.factory-default f 660 root wheel - - - - -
/etc/sysrepo/data/infix-shell-type.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-key-exchange-algs.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-tls-cipher-suite-algs.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-system.startup f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-netconf-with-defaults.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/iana-ssh-key-exchange-algs.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-factory-default.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/infix-dhcp-client.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ipv4-unicast-routing.running.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-ssh-server.operational.perm f 660 root wheel - - - - -
/etc/sysrepo/data/sysrepo-plugind.candidate.perm f 660 root wheel - - - - -
/etc/sysrepo/data/ietf-crypto-types.startup f 660 root wheel - - - - -
endef

define CONFD_CLEANUP
	rm -f /dev/shm/$(NETOPEER2_SYSREPO_SHM_PREFIX)*
endef
CONFD_POST_INSTALL_TARGET_HOOKS += CONFD_INSTALL_EXTRA
CONFD_POST_INSTALL_TARGET_HOOKS += CONFD_INSTALL_YANG_MODULES
CONFD_POST_INSTALL_TARGET_HOOKS += CONFD_CLEANUP
$(eval $(autotools-package))
