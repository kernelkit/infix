################################################################################
#
# osal
#
################################################################################

OSAL_VERSION = 0ab080859e3ba6d2b6d2fcac6f471d3c3d08d5f2
OSAL_SITE_METHOD = git
OSAL_SITE = https://github.com/rtlabs-com/osal
OSAL_GIT_SUBMODULES = YES
OSAL_INSTALL_STAGING = YES
OSAL_LICENSE = BSD-3-Clause
OSAL_LICENSE_FILES = LICENSE
OSAL_SUPPORTS_IN_SOURCE_BUILD = NO
OSAL_CONF_OPTS += \
	-DBUILD_SHARED_LIBS=true

$(eval $(cmake-package))
