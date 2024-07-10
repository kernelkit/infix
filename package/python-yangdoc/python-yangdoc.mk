################################################################################
#
# python-yangdoc
#
################################################################################

PYTHON_YANGDOC_VERSION = 0.6.0
PYTHON_YANGDOC_SOURCE = yangdoc-$(PYTHON_YANGDOC_VERSION).tar.gz
PYTHON_YANGDOC_SITE = https://files.pythonhosted.org/packages/a7/35/b9770953d7e42a6e2a2f0a88f79656610f4e36b33946cd957c5d7d55824d
PYTHON_YANGDOC_SETUP_TYPE = setuptools
PYTHON_YANGDOC_LICENSE = MIT
PYTHON_YANGDOC_LICENSE_FILES = LICENSE
HOST_PYTHON_YANGDOC_DEPENDENCIES = host-python-cython host-python-cffi host-python-libyang

$(eval $(host-python-package))
