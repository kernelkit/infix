################################################################################
#
# python-mako
#
################################################################################

PYTHON_TEMPLATING_VERSION = 1.1.5
PYTHON_TEMPLATING_SOURCE = Mako-$(PYTHON_TEMPLATING_VERSION).tar.gz
PYTHON_TEMPLATING_SITE = https://files.pythonhosted.org/packages/d1/42/ff293411e980debfc647be9306d89840c8b82ea24571b014f1a35b2ad80f
PYTHON_TEMPLATING_SETUP_TYPE = setuptools
PYTHON_TEMPLATING_LICENSE = MIT
PYTHON_TEMPLATING_LICENSE_FILES = LICENSE

# In host build, setup.py tries to download markupsafe if it is not installed
HOST_PYTHON_TEMPLATING_DEPENDENCIES = host-python-markupsafe

$(eval $(python-package))
$(eval $(host-python-package))
