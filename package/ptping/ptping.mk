################################################################################
#
# ptping
#
################################################################################

PTPING_VERSION = 0.1
PTPING_SITE = https://github.com/wkz/ptping/releases/download/v$(PTPING_VERSION)
PTPING_LICENSE = GPL-2.0
PTPING_LICENSE_FILES = COPYING

$(eval $(autotools-package))
