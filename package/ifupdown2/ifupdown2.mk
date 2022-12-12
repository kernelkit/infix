################################################################################
#
# ifupdown2
#
################################################################################

IFUPDOWN2_VERSION = a97d3e0cf96ab44e1e37a5940c83ea390f6656c7
IFUPDOWN2_SITE = $(call github,kernelkit,ifupdown2,$(IFUPDOWN2_VERSION))
IFUPDOWN2_LICENSE = GPL-2.0+
IFUPDOWN2_LICENSE_FILES = LICENSE
IFUPDOWN2_DEPENDENCIES = iproute2 python3 python-mako python-six python-setuptools
IFUPDOWN2_SETUP_TYPE = setuptools

$(eval $(python-package))
