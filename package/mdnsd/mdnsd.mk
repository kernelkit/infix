################################################################################
#
# mdnsd
#
################################################################################
MDNSD_VERSION = 0.11
MDNSD_SITE = https://github.com/troglobit/mdnsd/releases/download/v$(MDNSD_VERSION)
MDNSD_LICENSE = BSD-3-Clause
MDNSD_LICENSE_FILES = LICENSE
MDNSD_INSTALL_STAGING = YES

ifeq ($(BR2_PACKAGE_MDNSD_FTP_SERVICE),y)
define MDNSD_INSTALL_FTP_SERVICE
	$(INSTALL) -D -m 0644 $(BR2_EXTERNAL_INFIX_PATH)/package/mdnsd/ftp.service \
		$(TARGET_DIR)/etc/mdns.d/
endef
MDNSD_POST_INSTALL_TARGET_HOOKS += MDNSD_INSTALL_FTP_SERVICE
endif

ifeq ($(BR2_PACKAGE_MDNSD_HTTP_SERVICE),y)
define MDNSD_INSTALL_HTTP_SERVICE
	$(INSTALL) -D -m 0644 $(BR2_EXTERNAL_INFIX_PATH)/package/mdnsd/http.service \
		$(TARGET_DIR)/etc/mdns.d/
endef
MDNSD_POST_INSTALL_TARGET_HOOKS += MDNSD_INSTALL_HTTP_SERVICE
endif

ifeq ($(BR2_PACKAGE_MDNSD_IPP_SERVICE),y)
define MDNSD_INSTALL_IPP_SERVICE
	$(INSTALL) -D -m 0644 $(BR2_EXTERNAL_INFIX_PATH)/package/mdnsd/ipp.service \
		$(TARGET_DIR)/etc/mdns.d/
endef
MDNSD_POST_INSTALL_TARGET_HOOKS += MDNSD_INSTALL_IPP_SERVICE
endif

ifeq ($(BR2_PACKAGE_MDNSD_PRINTER_SERVICE),y)
define MDNSD_INSTALL_PRINTER_SERVICE
	$(INSTALL) -D -m 0644 $(BR2_EXTERNAL_INFIX_PATH)/package/mdnsd/printer.service \
		$(TARGET_DIR)/etc/mdns.d/
endef
MDNSD_POST_INSTALL_TARGET_HOOKS += MDNSD_INSTALL_PRINTER_SERVICE
endif

ifeq ($(BR2_PACKAGE_MDNSD_SSH_SERVICE),y)
define MDNSD_INSTALL_SSH_SERVICE
	$(INSTALL) -D -m 0644 $(BR2_EXTERNAL_INFIX_PATH)/package/mdnsd/ssh.service \
		$(TARGET_DIR)/etc/mdns.d/
endef
MDNSD_POST_INSTALL_TARGET_HOOKS += MDNSD_INSTALL_SSH_SERVICE
endif

define MDNSD_INSTALL_FINIT_SVC
	$(INSTALL) -D -m 0644 $(BR2_EXTERNAL_INFIX_PATH)/package/mdnsd/mdnsd.svc \
		$(FINIT_D)/available/mdnsd.conf
	$(INSTALL) -d -m 0755 $(FINIT_D)/enabled
	ln -sf ../available/mdnsd.conf $(FINIT_D)/enabled/mdnsd.conf
endef

MDNSD_POST_INSTALL_TARGET_HOOKS += MDNSD_INSTALL_FINIT_SVC

$(eval $(autotools-package))
