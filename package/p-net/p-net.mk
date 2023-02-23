################################################################################
#
# p-net
#
################################################################################

#P_NET_VERSION = a55fce3f0872bad0254040d8fa344f6a35f64d78
#P_NET_SITE = https://github.com/rtlabs-com/p-net
P_NET_VERSION = 6e4bc6d1842970fb33469c8998e3b4d11ee4c0e6
P_NET_SITE = https://github.com/addiva-elektronik/p-net
P_NET_SITE_METHOD = git
P_NET_GIT_SUBMODULES = YES
P_NET_INSTALL_STAGING = YES
P_NET_LICENSE = GPL-3.0
P_NET_LICENSE_FILES = LICENSE.md
P_NET_SUPPORTS_IN_SOURCE_BUILD = NO
P_NET_DEPENDENCIES = osal netsnmp
P_NET_CONF_OPTS += \
	-DBUILD_SHARED_LIBS=true \
	-DPNET_OPTION_SNMP=ON \
	-DPNET_MAX_SLOTS=$(BR2_PACKAGE_P_NET_MAX_SLOTS) \
	-DPNET_MAX_SUBSLOTS=$(BR2_PACKAGE_P_NET_MAX_SUBSLOTS) \
	-DPNET_MAX_PHYSICAL_PORTS=$(BR2_PACKAGE_P_NET_MAX_PHYSICAL_PORTS)

$(eval $(cmake-package))
