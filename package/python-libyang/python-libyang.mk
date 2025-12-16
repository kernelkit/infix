################################################################################
#
# python-libyang
#
################################################################################

PYTHON_LIBYANG_VERSION = 3.3.0
PYTHON_LIBYANG_SOURCE = libyang-$(PYTHON_LIBYANG_VERSION).tar.gz
PYTHON_LIBYANG_SITE = https://files.pythonhosted.org/packages/34/d8/ebe9de6d29eaf62a684b34d30118294e48a05d7fbd1bb0af002b7627140d
PYTHON_LIBYANG_SETUP_TYPE = setuptools
PYTHON_LIBYANG_LICENSE = MIT
PYTHON_LIBYANG_LICENSE_FILES = LICENSE
PYTHON_LIBYANG_DEPENDENCIES = python-cython python-cffi libyang
HOST_PYTHON_LIBYANG_DEPENDENCIES = host-python-cython host-python-cffi host-libyang

$(eval $(python-package))
$(eval $(host-python-package))
