################################################################################
#
# python-libyang
#
################################################################################

PYTHON_LIBYANG_VERSION = 5.1.0
PYTHON_LIBYANG_SOURCE = libyang-$(PYTHON_LIBYANG_VERSION).tar.gz
PYTHON_LIBYANG_SITE = https://files.pythonhosted.org/packages/ac/83/628bdfe71df4b47c82c4fdb1e866b125592a971c20adcf48ae496d5f4451
PYTHON_LIBYANG_SETUP_TYPE = setuptools
PYTHON_LIBYANG_LICENSE = MIT
PYTHON_LIBYANG_LICENSE_FILES = LICENSE
PYTHON_LIBYANG_DEPENDENCIES = python-cython python-cffi libyang
HOST_PYTHON_LIBYANG_DEPENDENCIES = host-python-cython host-python-cffi host-libyang

$(eval $(python-package))
$(eval $(host-python-package))
