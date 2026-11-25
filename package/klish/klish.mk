################################################################################
#
# klish
#
################################################################################

KLISH_VERSION = 5a42de6e3b11348414a1dd1d06c21112b120fb65
KLISH_SITE = https://github.com/kernelkit/klish.git
#KLISH_VERSION = tags/3.0.0
#KLISH_SITE = https://src.libcode.org/pkun/klish.git
KLISH_SITE_METHOD = git
KLISH_LICENSE = BSD-3
KLISH_LICENSE_FILES = LICENCE
KLISH_DEPENDENCIES = faux libxml2
KLISH_INSTALL_STAGING = YES
KLISH_AUTORECONF = YES

KLISH_CONF_OPTS += --with-libxml2

define KLISH_INSTALL_CONFIG
	$(INSTALL) -t $(TARGET_DIR)/etc/klish -D -m 0644 \
		$(@D)/plugins/klish/xml/ptypes.xml
	$(INSTALL) -t $(TARGET_DIR)/etc/klish -m 0644 $(KLISH_PKGDIR)/klish.conf
	$(INSTALL) -t $(TARGET_DIR)/etc/klish -m 0644 $(KLISH_PKGDIR)/klishd.conf
endef
KLISH_POST_INSTALL_TARGET_HOOKS += KLISH_INSTALL_CONFIG

ifeq ($(BR2_PACKAGE_KLISH_DEFAULT_XML),y)
define KLISH_INSTALL_XML
	$(INSTALL) -t $(TARGET_DIR)/etc/klish -m 0644 $(KLISH_PKGDIR)/default.xml
endef
KLISH_POST_INSTALL_TARGET_HOOKS += KLISH_INSTALL_XML
endif

# The klish client starts klishd for the session, see klish.conf
define KLISH_PERMISSIONS
	/usr/bin/klishd  f 04750 root klish - - - - -
endef

$(eval $(autotools-package))
