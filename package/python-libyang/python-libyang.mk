################################################################################
#
# python-libyang
#
################################################################################

PYTHON_LIBYANG_VERSION = 3.0.1
PYTHON_LIBYANG_SOURCE = libyang-$(PYTHON_LIBYANG_VERSION).tar.gz
PYTHON_LIBYANG_SITE = https://files.pythonhosted.org/packages/91/2e/ff13ee16c874d232d5d3fdff83f629cbc9ac47f9aaf2b59256b6a1bdbc16
PYTHON_LIBYANG_SETUP_TYPE = setuptools
PYTHON_LIBYANG_LICENSE = MIT
PYTHON_LIBYANG_LICENSE_FILES = LICENSE
PYTHON_LIBYANG_DEPENDENCIES = python-cython python-cffi
HOST_PYTHON_LIBYANG_DEPENDENCIES = host-python-cython host-python-cffi

$(eval $(python-package))
$(eval $(host-python-package))
