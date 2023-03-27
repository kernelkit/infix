################################################################################
#
# skeleton-init-finit
#
################################################################################

# The skeleton can't depend on the toolchain, since all packages depends on the
# skeleton and the toolchain is a target package, as is skeleton.
# Hence, skeleton would depends on the toolchain and the toolchain would depend
# on skeleton.
SKELETON_INIT_FINIT_ADD_TOOLCHAIN_DEPENDENCY = NO
SKELETON_INIT_FINIT_ADD_SKELETON_DEPENDENCY = NO
SKELETON_INIT_FINIT_DEPENDENCIES = skeleton-init-common
SKELETON_INIT_FINIT_AVAILABLE = $(SKELETON_INIT_FINIT_PKGDIR)/skeleton/etc/finit.d/available
# Enable when BR2_INIT_FINT
#SKELETON_INIT_FINIT_PROVIDES = skeleton

# Prefer Finit built-in getty unless options are set, squash zero baudrate
define SKELETON_INIT_FINIT_GETTY
	if [ -z "$(SYSTEM_GETTY_OPTIONS)" ]; then \
		if [ $(SYSTEM_GETTY_BAUDRATE) -eq 0 ]; then \
			SYSTEM_GETTY_BAUDRATE=""; \
		fi; \
		echo "tty [12345789] $(SYSTEM_GETTY_PORT) $(SYSTEM_GETTY_BAUDRATE) $(SYSTEM_GETTY_TERM) noclear passenv"; \
	else \
		echo "tty [12345789] /sbin/getty -L $(SYSTEM_GETTY_OPTIONS) $(SYSTEM_GETTY_BAUDRATE) $(SYSTEM_GETTY_PORT) $(SYSTEM_GETTY_TERM)"; \
	fi
endef

define SKELETON_INIT_FINIT_SET_GENERIC_GETTY
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/getty.conf $(FINIT_D)/available/
	$(SKELETON_INIT_FINIT_GETTY) >> $(FINIT_D)/available/getty.conf
	ln -sf ../available/getty.conf $(FINIT_D)/enabled/getty.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_GENERIC_GETTY

# Avahi mDNS-SD
ifeq ($(BR2_PACKAGE_AVAHI_DAEMON),y)
define SKELETON_INIT_FINIT_SET_AVAHI
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/avahi.conf $(FINIT_D)/available/
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/avahi-dnsconfd.conf $(FINIT_D)/available/
	ln -sf ../available/avahi.conf $(FINIT_D)/enabled/avahi.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_AVAHI
endif

ifeq ($(BR2_PACKAGE_CHRONY),y)
define SKELETON_INIT_FINIT_SET_CHRONY
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/chronyd.conf $(FINIT_D)/available/
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_CHRONY
endif

ifeq ($(BR2_PACKAGE_CONNTRACKD),y)
define SKELETON_INIT_FINIT_SET_CONNTRACKD
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/conntrackd.conf $(FINIT_D)/available/
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_CONNTRACKD
endif

ifeq ($(BR2_PACKAGE_DNSMASQ),y)
define SKELETON_INIT_FINIT_SET_DNSMASQ
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/dnsmasq.conf $(FINIT_D)/available/
	ln -sf ../available/dnsmasq.conf $(FINIT_D)/enabled/dnsmasq.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_DNSMASQ
endif

# Dropbear SSH
ifeq ($(BR2_PACKAGE_DROPBEAR),y)
define SKELETON_INIT_FINIT_SET_DROPBEAR
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/dropbear.conf $(FINIT_D)/available/
	ln -sf ../available/dropbear.conf $(FINIT_D)/enabled/dropbear.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_DROPBEAR
endif

ifeq ($(BR2_PACKAGE_FRR),y)
define SKELETON_INIT_FINIT_SET_FRR
	for svc in babeld bfdd bgpd eigrpd isisd ldpd ospfd ospf6d pathd ripd ripng staticd vrrpd zebra; do	\
		cp $(SKELETON_INIT_FINIT_AVAILABLE)/frr/$$svc.conf $(FINIT_D)/available/$$svc.conf;		\
	done
	ln -sf ../available/zebra.conf $(FINIT_D)/enabled/zebra.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_FRR
endif # BR2_PACKAGE_FRR

ifeq ($(BR2_PACKAGE_INADYN),y)
define SKELETON_INIT_FINIT_SET_INADYN
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/inadyn.conf $(FINIT_D)/available/
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_INADYN
endif

ifeq ($(BR2_PACKAGE_LLDPD),y)
define SKELETON_INIT_FINIT_SET_LLDPD
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/lldpd.conf $(FINIT_D)/available/
	ln -sf ../available/lldpd.conf $(FINIT_D)/enabled/lldpd.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_LLDPD
endif

ifeq ($(BR2_PACKAGE_MSTPD),y)
define SKELETON_INIT_FINIT_SET_MSTPD
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/mstpd.conf $(FINIT_D)/available/
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_MSTPD
endif

ifeq ($(BR2_PACKAGE_NETSNMP),y)
define SKELETON_INIT_FINIT_SET_NETSNMP
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/snmpd.conf $(FINIT_D)/available/
	ln -sf ../available/snmpd.conf $(FINIT_D)/enabled/snmpd.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_NETSNMP
endif

ifeq ($(BR2_PACKAGE_NGINX),y)
define SKELETON_INIT_FINIT_SET_NGINX
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/nginx.conf $(FINIT_D)/available/
	ln -sf ../available/nginx.conf $(FINIT_D)/enabled/nginx.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_NGINX
endif

ifeq ($(BR2_PACKAGE_NTPD),y)
define SKELETON_INIT_FINIT_SET_NTPD
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/ntpd.conf $(FINIT_D)/available/
	ln -sf ../available/ntpd.conf $(FINIT_D)/enabled/ntpd.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_NTPD
endif

ifeq ($(BR2_PACKAGE_MINI_SNMPD),y)
define SKELETON_INIT_FINIT_SET_MINI_SNMPD
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/mini-snmpd.conf $(FINIT_D)/available/
	ln -sf ../available/mini-snmpd.conf $(FINIT_D)/enabled/mini-snmpd.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_MINI_SNMPD
endif

# OpenSSH
ifeq ($(BR2_PACKAGE_OPENSSH),y)
define SKELETON_INIT_FINIT_SET_OPENSSH
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/sshd.conf $(FINIT_D)/available/
	ln -sf ../available/sshd.conf $(FINIT_D)/enabled/sshd.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_OPENSSH
endif

ifeq ($(BR2_PACKAGE_PROFETH),y)
define SKELETON_INIT_FINIT_SET_PROFETH
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/profeth.conf $(FINIT_D)/available/
	ln -sf ../available/profeth.conf $(FINIT_D)/enabled/
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_PROFETH
endif

ifeq ($(BR2_PACKAGE_QUAGGA),y)
define SKELETON_INIT_FINIT_SET_QUAGGA
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/quagga/zebra.conf $(FINIT_D)/available/
	ln -sf ../available/zebra.conf $(FINIT_D)/enabled/zebra.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_QUAGGA

ifeq ($(BR2_PACKAGE_QUAGGA_ISISD),y)
define SKELETON_INIT_FINIT_SET_QUAGGA_ISISD
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/quagga/isisd.conf $(FINIT_D)/available/
	ln -sf ../available/isisd.conf $(FINIT_D)/enabled/isisd.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_QUAGGA_ISISD
endif

ifeq ($(BR2_PACKAGE_QUAGGA_OSPFD),y)
define SKELETON_INIT_FINIT_SET_QUAGGA_OSPFD
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/quagga/ospfd.conf $(FINIT_D)/available/
	ln -sf ../available/ospfd.conf $(FINIT_D)/enabled/ospfd.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_QUAGGA_OSPFD
endif

ifeq ($(BR2_PACKAGE_QUAGGA_OSP6D),y)
define SKELETON_INIT_FINIT_SET_QUAGGA_OSP6D
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/quagga/ospf6d.conf $(FINIT_D)/available/
	ln -sf ../available/ospf6d.conf $(FINIT_D)/enabled/ospf6d.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_QUAGGA_OSP6D
endif

ifeq ($(BR2_PACKAGE_QUAGGA_RIPD),y)
define SKELETON_INIT_FINIT_SET_QUAGGA_RIPD
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/quagga/ripd.conf $(FINIT_D)/available/
	ln -sf ../available/ripd.conf $(FINIT_D)/enabled/ripd.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_QUAGGA_RIPD
endif

ifeq ($(BR2_PACKAGE_QUAGGA_RIPNGD),y)
define SKELETON_INIT_FINIT_SET_QUAGGA_RIPNG
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/quagga/ripng.conf $(FINIT_D)/available/
	ln -sf ../available/ripng.conf $(FINIT_D)/enabled/ripng.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_QUAGGA_RIPNG
endif

endif # BR2_PACKAGE_QUAGGA

ifeq ($(BR2_PACKAGE_SMCROUTE),y)
define SKELETON_INIT_FINIT_SET_SMCROUTE
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/smcroute.conf $(FINIT_D)/available/
	ln -sf ../available/smcroute.conf $(FINIT_D)/enabled/smcroute.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_SMCROUTE
endif

# SSDP Responder
ifeq ($(BR2_PACKAGE_SSDP_RESPONDER),y)
define SKELETON_INIT_FINIT_SET_SSDP_RESPONDER
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/ssdpd.conf $(FINIT_D)/available/
	ln -sf ../available/ssdpd.conf $(FINIT_D)/enabled/ssdpd.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_SSDP_RESPONDER
endif

# Enable Busybox syslogd unless sysklogd is enabled
ifeq ($(BR2_PACKAGE_SYSKLOGD),y)
define SKELETON_INIT_FINIT_SET_SYSLOGD
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/sysklogd.conf $(FINIT_D)/available/
	ln -sf ../available/sysklogd.conf $(FINIT_D)/enabled/sysklogd.conf
	rm -f $(FINIT_D)/enabled/syslogd.conf
endef
else
define SKELETON_INIT_FINIT_SET_SYSLOGD
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/syslogd.conf $(FINIT_D)/available/
	ln -sf ../available/syslogd.conf $(FINIT_D)/enabled/syslogd.conf
	rm -f $(FINIT_D)/enabled/sysklogd.conf
endef
endif
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_SYSLOGD

ifeq ($(BR2_PACKAGE_ULOGD),y)
define SKELETON_INIT_FINIT_SET_ULOGD
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/ulogd.conf $(FINIT_D)/available/
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_ULOGD
endif

ifeq ($(BR2_PACKAGE_WATCHDOGD),y)
define SKELETON_INIT_FINIT_SET_WATCHDOGD
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/watchdogd.conf $(FINIT_D)/available/
	ln -sf ../available/watchdogd.conf $(FINIT_D)/enabled/watchdogd.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_WATCHDOGD
endif

# Enable gdbserver when running in Qemu mode
ifeq ($(QEMU_GDB),y)
define SKELETON_INIT_FINIT_SET_GDBSERVER
	cp $(SKELETON_INIT_FINIT_AVAILABLE)/gdbserver.conf $(FINIT_D)/available/
	ln -sf ../available/gdbserver.conf $(FINIT_D)/enabled/gdbserver.conf
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_SET_GDBSERVER
endif

# Workaround, should be in ifupdown-scripts package
ifeq ($(BR2_PACKAGE_IFUPDOWN_SCRIPTS),y)
define SKELETON_INIT_FINIT_IFUPDOWN_WORKAROUND
	$(IFUPDOWN_SCRIPTS_PREAMBLE)
	$(IFUPDOWN_SCRIPTS_LOCALHOST)
        $(IFUPDOWN_SCRIPTS_DHCP)
endef
SKELETON_INIT_FINIT_TARGET_FINALIZE_HOOKS += SKELETON_INIT_FINIT_IFUPDOWN_WORKAROUND
endif


ifeq ($(BR2_TARGET_GENERIC_REMOUNT_ROOTFS_RW),y)
# Uncomment /dev/root entry in fstab to allow Finit to remount it rw
define SKELETON_INIT_FINIT_ROOT_RO_OR_RW
	$(SED) '\:^#[[:blank:]]*/dev/root[[:blank:]]:s/^# //' $(TARGET_DIR)/etc/fstab
endef
else
# Comment out /dev/root entry to prevent Finit from remounting it rw
define SKELETON_INIT_FINIT_ROOT_RO_OR_RW
	$(SED) '\:^/dev/root[[:blank:]]:s/^/# /' $(TARGET_DIR)/etc/fstab
endef
endif

ifeq ($(BR2_PACKAGE_SKELETON_INIT_FINIT_TELNETD),y)
define SKELETON_INIT_FINIT_TELNETD
	$(SED) '\:^#[[:blank:]]*telnet[[:blank:]]:s/^# //' $(TARGET_DIR)/etc/inetd.conf
endef
else
define SKELETON_INIT_FINIT_TELNETD
	$(SED) '\:^telnet[[:blank:]]:s/^/# /' $(TARGET_DIR)/etc/inetd.conf
endef
endif

define SKELETON_INIT_FINIT_INSTALL_TARGET_CMDS
	$(call SYSTEM_RSYNC,$(SKELETON_INIT_FINIT_PKGDIR)/skeleton,$(TARGET_DIR))
	rm -rf $(TARGET_DIR)/etc/finit.d/available $(TARGET_DIR)/etc/finit.d/enabled
	mkdir -p $(TARGET_DIR)/etc/finit.d/available
	mkdir -p $(TARGET_DIR)/etc/finit.d/enabled
	for svc in getty inetd ntpd telnetd; do \
		cp $(SKELETON_INIT_FINIT_AVAILABLE)/$$svc.conf $(FINIT_D)/available/; \
	done
	mkdir -p $(TARGET_DIR)/home
	mkdir -p $(TARGET_DIR)/srv
	mkdir -p $(TARGET_DIR)/var
	[ -L $(TARGET_DIR)/var/run ] || ln -s ../run $(TARGET_DIR)/var/run
	$(SKELETON_INIT_FINIT_ROOT_RO_OR_RW)
	$(SKELETON_INIT_FINIT_TELNETD)
endef

$(eval $(generic-package))
