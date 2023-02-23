################################################################################
#
# profeth
#
################################################################################
PROFETH_VERSION = 1.0
PROFETH_LICENSE = GPL-3.0
PROFETH_SITE_METHOD = local
PROFETH_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/profeth
PROFETH_DEPENDENCIES = p-net
PROFETH_AUTORECONF = YES

define PROFETH_POST_RSYNC_INSTALL_OPTS
	@echo Installing generated options.h from p-net
	cp $(BUILD_DIR)/p-net*/buildroot-build/src/options.h $(@D)/src/options.h
endef
PROFETH_POST_RSYNC_HOOKS += PROFETH_POST_RSYNC_INSTALL_OPTS

$(eval $(autotools-package))
