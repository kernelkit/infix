################################################################################
#
# python-yangdoc
#
################################################################################

PYTHON_YANGDOC_VERSION = 0.4.0
PYTHON_YANGDOC_SOURCE = yangdoc-$(PYTHON_YANGDOC_VERSION).tar.gz
PYTHON_YANGDOC_SITE = https://files.pythonhosted.org/packages/75/83/a426dcb6f9c56b3cfc70c49040a7ea76c6e9be8ed86026eb6b5942dcb03a
PYTHON_YANGDOC_SETUP_TYPE = setuptools
PYTHON_YANGDOC_LICENSE = MIT
PYTHON_YANGDOC_LICENSE_FILES = LICENSE
HOST_PYTHON_LIBYANG_DEPENDENCIES = host-python-cython host-python-cffi host-python-libyang

$(eval $(host-python-package))
