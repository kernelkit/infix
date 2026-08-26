################################################################################
#
# mbedtls-atf
#
# mbed TLS sources for Arm Trusted Firmware, which needs them to parse
# the X.509 certificates in a FIP when a platform has Trusted Board Boot,
# as Microchip LAN969x (Laguna) does.  ATF compiles them into BL1 and BL2
# itself, so there is nothing to build or install here.
#
# The mbedtls package cannot be used for this: it tracks 3.x, while ATF
# 2.8 builds only against the 2.28 LTS series.
#
################################################################################

MBEDTLS_ATF_VERSION = 2.28.5
MBEDTLS_ATF_SITE = $(call github,Mbed-TLS,mbedtls,mbedtls-$(MBEDTLS_ATF_VERSION))
MBEDTLS_ATF_LICENSE = Apache-2.0
MBEDTLS_ATF_LICENSE_FILES = LICENSE
MBEDTLS_ATF_INSTALL_TARGET = NO

$(eval $(generic-package))

ifeq ($(BR2_PACKAGE_MBEDTLS_ATF),y)
# MAKE_OPTS is expanded when the ATF build step runs, so appending to it
# from here works even though Buildroot reads this file after the package
# was defined.  DEPENDENCIES is read at definition time and cannot be
# appended to, hence the explicit ordering.
ARM_TRUSTED_FIRMWARE_MAKE_OPTS += MBEDTLS_DIR=$(MBEDTLS_ATF_DIR)
$(ARM_TRUSTED_FIRMWARE_TARGET_BUILD): $(MBEDTLS_ATF_TARGET_EXTRACT)
endif
