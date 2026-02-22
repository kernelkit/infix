################################################################################
#
# initviz
#
################################################################################

INITVIZ_VERSION = 1.0.0-rc1
INITVIZ_SITE = https://github.com/finit-project/InitViz/releases/download/$(INITVIZ_VERSION)
INITVIZ_SOURCE = initviz-$(INITVIZ_VERSION).tar.gz
INITVIZ_LICENSE = GPL-2.0-or-later
INITVIZ_LICENSE_FILES = COPYING

# Target package: bootchartd collector daemon
define INITVIZ_BUILD_CMDS
	$(TARGET_MAKE_ENV) $(TARGET_CONFIGURE_OPTS) \
		$(MAKE) -C $(@D) collector
endef

define INITVIZ_INSTALL_TARGET_CMDS
	$(TARGET_MAKE_ENV) $(MAKE) -C $(@D) \
		DESTDIR=$(TARGET_DIR) \
		EARLY_PREFIX= \
		install-collector
endef

$(eval $(generic-package))
