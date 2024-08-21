################################################################################
#
# klish
#
################################################################################

KLISH_VERSION = 894b0ba24ffcb5072b4b18f3b5aee3500d05cd86
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

define KLISH_INSTALL_FINIT_SVC
	$(INSTALL) -D -m 0644 $(BR2_EXTERNAL_INFIX_PATH)/package/klish/klish.svc \
		$(FINIT_D)/available/klish.conf
	$(INSTALL) -d -m 0755 $(FINIT_D)/enabled
	ln -sf ../available/klish.conf $(FINIT_D)/enabled/klish.conf
endef

KLISH_POST_INSTALL_TARGET_HOOKS += KLISH_INSTALL_FINIT_SVC

$(eval $(autotools-package))
