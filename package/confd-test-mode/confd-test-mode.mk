################################################################################
#
# confd-test-mode
#
################################################################################

CONFD_TEST_MODE_VERSION = 1.0
CONFD_TEST_MODE_SITE_METHOD = local
CONFD_TEST_MODE_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/test-mode
CONFD_TEST_MODE_LICENSE = BSD-3-Clause
CONFD_TEST_MODE_LICENSE_FILES = LICENSE
CONFD_TEST_MODE_REDISTRIBUTE = NO
CONFD_TEST_MODE_DEPENDENCIES = sysrepo libite libyang confd
CONFD_TEST_MODE_AUTORECONF = YES

$(eval $(autotools-package))
