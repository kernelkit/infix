################################################################################
#
# lowdown
#
################################################################################

LOWDOWN_VERSION = VERSION_1_0_2
LOWDOWN_SITE = $(call github,kristapsdz,lowdown,$(LOWDOWN_VERSION))
LOWDOWN_LICENSE = ISC
LOWDOWN_LICENSE_FILES = LICENSE.md

define LOWDOWN_CONFIGURE_CMDS
        (cd $(@D); $(TARGET_CONFIGURE_OPTS) ./configure)
endef

define LOWDOWN_BUILD_CMDS
        $(TARGET_MAKE_ENV) $(MAKE) -C $(@D)
endef

define LOWDOWN_INSTALL_TARGET_CMDS
	$(INSTALL) -t $(TARGET_DIR)/usr/bin -D -m 0755 $(@D)/lowdown
endef

$(eval $(generic-package))
