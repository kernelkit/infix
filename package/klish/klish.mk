################################################################################
#
# klish
#
################################################################################

KLISH_VERSION = 2a1773aaf9d48932f7b4be79757f4011729b3432
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

# klish starts klishd for the session, only group klish gets into the directory
define KLISH_INSTALL_SERVER
	mkdir -p $(TARGET_DIR)/usr/libexec/klish
	mv $(TARGET_DIR)/usr/bin/klishd $(TARGET_DIR)/usr/libexec/klish/
endef
KLISH_POST_INSTALL_TARGET_HOOKS += KLISH_INSTALL_SERVER

define KLISH_PERMISSIONS
	/usr/libexec/klish         d 0750  root klish   - - - - -
	/usr/libexec/klish/klishd  f 02755 root sysrepo - - - - -
endef

$(eval $(autotools-package))
