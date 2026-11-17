################################################################################
#
# webui
#
################################################################################

WEBUI_VERSION = 1.0
WEBUI_SITE_METHOD = local
WEBUI_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/webui
WEBUI_GOMOD = infix/webui
WEBUI_LICENSE = MIT
WEBUI_LICENSE_FILES = LICENSE
WEBUI_REDISTRIBUTE = NO

define WEBUI_INSTALL_EXTRA
	$(INSTALL) -D -m 0644 $(WEBUI_PKGDIR)/webui.svc \
		$(FINIT_D)/available/webui.conf
	$(INSTALL) -D -m 0644 $(WEBUI_PKGDIR)/webui.conf \
		$(TARGET_DIR)/etc/nginx/app/webui.conf
	$(INSTALL) -D -m 0644 $(WEBUI_PKGDIR)/webui-proxy.conf \
		$(TARGET_DIR)/etc/nginx/webui-proxy.conf
	$(INSTALL) -D -m 0644 $(WEBUI_PKGDIR)/default.conf \
		$(TARGET_DIR)/etc/nginx/available/default.conf
	$(INSTALL) -D -m 0644 $(WEBUI_PKGDIR)/50x.html \
		$(TARGET_DIR)/usr/html/50x.html
endef
WEBUI_POST_INSTALL_TARGET_HOOKS += WEBUI_INSTALL_EXTRA

# The schema is compiled on the build machine, by webui itself
define WEBUI_BUILD_HOST
	cd $(@D); $(HOST_GO_HOST_ENV) \
		$(GO_BIN) build -o $(@D)/bin/webui-host $(WEBUI_GOMOD)
endef
WEBUI_POST_BUILD_HOOKS += WEBUI_BUILD_HOST

# Run when every package has installed its YANG modules in sysrepo
define WEBUI_COMPILE_SCHEMA
	$(WEBUI_DIR)/bin/webui-host \
		-compile-schema $(TARGET_DIR)/etc/sysrepo/yang \
		-schema $(TARGET_DIR)/usr/share/webui/schema.json
endef
WEBUI_TARGET_FINALIZE_HOOKS += WEBUI_COMPILE_SCHEMA

$(eval $(golang-package))
