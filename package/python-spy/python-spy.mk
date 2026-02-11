################################################################################
#
# python-spy
#
################################################################################

PYTHON_SPY_VERSION = 0.4.1

ifeq ($(BR2_aarch64),y)
PYTHON_SPY_SITE = https://files.pythonhosted.org/packages/df/79/9ed50bb0a9de63ed023aa2db8b6265b04a7760d98c61eb54def6a5fddb68
PYTHON_SPY_SOURCE = py_spy-$(PYTHON_SPY_VERSION)-py2.py3-none-manylinux_2_17_aarch64.manylinux2014_aarch64.whl
else ifeq ($(BR2_x86_64),y)
PYTHON_SPY_SITE = https://files.pythonhosted.org/packages/68/fb/bc7f639aed026bca6e7beb1e33f6951e16b7d315594e7635a4f7d21d63f4
PYTHON_SPY_SOURCE = py_spy-$(PYTHON_SPY_VERSION)-py2.py3-none-manylinux_2_5_x86_64.manylinux1_x86_64.whl
endif

PYTHON_SPY_LICENSE = MIT
PYTHON_SPY_DEPENDENCIES = host-python3 host-python-installer

# Keep the wheel intact; we install from it directly.
define PYTHON_SPY_EXTRACT_CMDS
	cp $(PYTHON_SPY_DL_DIR)/$(PYTHON_SPY_SOURCE) $(@D)/
endef

define PYTHON_SPY_INSTALL_TARGET_CMDS
	$(HOST_DIR)/bin/python3 $(TOPDIR)/support/scripts/pyinstaller.py \
		$(@D)/$(PYTHON_SPY_SOURCE) \
		--interpreter=/usr/bin/python3 \
		--script-kind=posix \
		--purelib=$(TARGET_DIR)/usr/lib/python$(PYTHON3_VERSION_MAJOR)/site-packages \
		--headers=$(TARGET_DIR)/usr/include/python$(PYTHON3_VERSION_MAJOR) \
		--scripts=$(TARGET_DIR)/usr/bin \
		--data=$(TARGET_DIR)
endef

$(eval $(generic-package))
