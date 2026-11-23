################################################################################
#
# dhcprelayd
#
################################################################################

DHCPRELAYD_VERSION = 3.0-rc2
DHCPRELAYD_SITE    = https://github.com/kernelkit/dhcprelayd/releases/download/v$(DHCPRELAYD_VERSION)
DHCPRELAYD_LICENSE = GPL-2.0 or GPL-3.0
DHCPRELAYD_LICENSE_FILES = COPYING
DHCPRELAYD_DEPENDENCIES = host-pkgconf libconfuse libev
DHCPRELAYD_CONF_OPTS = --disable-silent-rules

define DHCPRELAYD_INSTALL_FINIT_SVC
	$(INSTALL) -D -m 0644 $(DHCPRELAYD_PKGDIR)/dhcprelayd.svc \
		$(FINIT_D)/available/dhcprelayd.conf
endef

DHCPRELAYD_POST_INSTALL_TARGET_HOOKS += DHCPRELAYD_INSTALL_FINIT_SVC

$(eval $(autotools-package))
