################################################################################
#
# ifupdown2
#
################################################################################

IFUPDOWN2_VERSION = db0f7548189dde918bfc37dd864cf935d5384a83
IFUPDOWN2_SITE = $(call github,kernelkit,ifupdown2,$(IFUPDOWN2_VERSION))
IFUPDOWN2_LICENSE = GPL-2.0+
IFUPDOWN2_LICENSE_FILES = LICENSE
IFUPDOWN2_DEPENDENCIES = iproute2 python3 python-templating python-six python-setuptools
IFUPDOWN2_SETUP_TYPE = setuptools

$(eval $(python-package))
