################################################################################
#
# webui
#
################################################################################

WEBUI_VERSION = 1.0
WEBUI_SITE_METHOD = local
WEBUI_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/webui
WEBUI_GOMOD = github.com/kernelkit/webui
WEBUI_LICENSE = MIT
WEBUI_LICENSE_FILES = LICENSE
WEBUI_REDISTRIBUTE = NO

define WEBUI_INSTALL_EXTRA
	$(INSTALL) -D -m 0644 $(WEBUI_PKGDIR)/webui.svc \
		$(FINIT_D)/available/webui.conf
	$(INSTALL) -D -m 0644 $(WEBUI_PKGDIR)/webui.conf \
		$(TARGET_DIR)/etc/nginx/app/webui.conf
endef
WEBUI_POST_INSTALL_TARGET_HOOKS += WEBUI_INSTALL_EXTRA

$(eval $(golang-package))
