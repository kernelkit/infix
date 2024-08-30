TEST_MODE_VERSION = 1.0
TEST_MODE_SITE_METHOD = local
TEST_MODE_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/test-mode
TEST_MODE_LICENSE = BSD-3-Clause
TEST_MODE_LICENSE_FILES = LICENSE
TEST_MODE_REDISTRIBUTE = NO
TEST_MODE_DEPENDENCIES = sysrepo libite libyang confd
TEST_MODE_AUTORECONF = YES
TEST_MODE_SYSREPO_SHM_PREFIX = sr_buildroot$(subst /,_,$(CONFIG_DIR))_test_mode

COMMON_SYSREPO_ENV = \
	SYSREPO_SHM_PREFIX=$(CONFD_SYSREPO_SHM_PREFIX) \
	SYSREPOCTL_EXECUTABLE="$(HOST_DIR)/bin/sysrepoctl" \
	SYSREPOCFG_EXECUTABLE="$(HOST_DIR)/bin/sysrepocfg" \
	SEARCH_PATH="$(TARGET_DIR)/usr/share/yang/modules/confd/"

define TEST_MODE_INSTALL_YANG_MODULES
        $(COMMON_SYSREPO_ENV) \
        SEARCH_PATH="$(TARGET_DIR)/usr/share/yang/modules/test-mode/" $(BR2_EXTERNAL_INFIX_PATH)/utils/sysrepo-load-modules.sh $(@D)/yang/test-mode.inc
endef
define TEST_MODE_PERMISSIONS
	/etc/sysrepo/data/ r 660 root wheel - - - - -
	/etc/sysrepo/data  d 770 root wheel - - - - -
endef
define TEST_MODE_CLEANUP
        rm -f /dev/shm/$(TEST_MODE_SYSREPO_SHM_PREFIX)*
endef

TEST_MODE_PRE_INSTALL_TARGET_HOOKS += TEST_MODE_CLEANUP
TEST_MODE_POST_INSTALL_TARGET_HOOKS += TEST_MODE_INSTALL_YANG_MODULES

$(eval $(autotools-package))
