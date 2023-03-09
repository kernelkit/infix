################################################################################
#
# profeth
#
################################################################################

PROFETH_VERSION = 1.0
PROFETH_LICENSE = GPL-3.0
PROFETH_SITE_METHOD = local
PROFETH_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/profeth
PROFETH_DEPENDENCIES = p-net
PROFETH_AUTORECONF = YES

# Ugly workaround to be able to build standalone applications using the
# p-net library.  This should probably be addressed with upstream.
define PROFETH_POST_RSYNC_INSTALL_OPTS
	@echo "*** Installing pnal headers and generated options.h from p-net"
	cp $(P_NET_SRCDIR)/src/ports/linux/pnal_sys.h $(@D)/src/
	cp $(P_NET_SRCDIR)/src/pnal.h                 $(@D)/src/
	cp $(P_NET_BUILDDIR)/src/options.h            $(@D)/src/
endef
PROFETH_POST_RSYNC_HOOKS += PROFETH_POST_RSYNC_INSTALL_OPTS

define PROFETH_INSTALL_EXTRA
	cp $(PROFETH_PKGDIR)/sysctl.conf $(TARGET_DIR)/etc/sysctl.d/profeth.conf
	cp $(PROFETH_PKGDIR)/set_profinet_leds $(TARGET_DIR)/usr/sbin/
	cp $(PROFETH_PKGDIR)/set_network_parameters $(TARGET_DIR)/usr/sbin/
	mkdir -p $(TARGET_DIR)/etc/snmp
	mkdir -p $(TARGET_DIR)/etc/tmpfiles.d
	mkdir -p $(TARGET_DIR)/etc/default
	cp $(PROFETH_PKGDIR)/snmpd.conf $(TARGET_DIR)/etc/snmp/
	cp $(PROFETH_PKGDIR)/tmpfiles.conf $(TARGET_DIR)/etc/tmpfiles.d/profeth.conf
	cp $(PROFETH_PKGDIR)/default $(TARGET_DIR)/etc/default/profeth
endef
PROFETH_TARGET_FINALIZE_HOOKS += PROFETH_INSTALL_EXTRA

$(eval $(autotools-package))
