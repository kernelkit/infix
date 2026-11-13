################################################################################
#
# skopeo-target
#
################################################################################

SKOPEO_TARGET_VERSION = 1.18.0
SKOPEO_TARGET_SITE = $(call github,containers,skopeo,v$(SKOPEO_TARGET_VERSION))
SKOPEO_TARGET_SOURCE = skopeo-$(SKOPEO_TARGET_VERSION)-go2.tar.gz
SKOPEO_TARGET_LICENSE = Apache-2.0
SKOPEO_TARGET_LICENSE_FILES = LICENSE
SKOPEO_TARGET_CPE_ID_VALID = YES

SKOPEO_TARGET_BUILD_TARGETS = cmd/skopeo
# OpenPGP in Go, no gpgme; no storage drivers, we only copy to OCI layouts
SKOPEO_TARGET_TAGS = containers_image_openpgp exclude_graphdriver_btrfs \
	exclude_graphdriver_devicemapper exclude_graphdriver_overlay

define SKOPEO_TARGET_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0755 $(@D)/bin/skopeo $(TARGET_DIR)/usr/bin/skopeo
endef

$(eval $(golang-package))
