################################################################################
#
# yangerd
#
################################################################################

YANGERD_VERSION = 1.0.0
YANGERD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/yangerd
YANGERD_SITE_METHOD = local
YANGERD_GOMOD = github.com/kernelkit/infix/src/yangerd
YANGERD_LICENSE = BSD-3-Clause
YANGERD_LICENSE_FILES = LICENSE
YANGERD_REDISTRIBUTE = NO

YANGERD_BUILD_TARGETS = cmd/yangerd cmd/yangerctl
YANGERD_INSTALL_BINS = yangerd yangerctl

define YANGERD_INSTALL_EXTRA
	$(INSTALL) -D -m 0644 $(YANGERD_PKGDIR)/yangerd.conf \
		$(FINIT_D)/available/yangerd.conf
	ln -sf ../available/yangerd.conf $(FINIT_D)/enabled/yangerd.conf
	$(INSTALL) -D -m 0644 $(YANGERD_PKGDIR)/yangerd.default \
		$(TARGET_DIR)/etc/default/yangerd
endef
YANGERD_POST_INSTALL_TARGET_HOOKS += YANGERD_INSTALL_EXTRA

$(eval $(golang-package))
