################################################################################
#
# spacemit-k1-fsbl
#
################################################################################

SPACEMIT_K1_FSBL_VERSION = 78fda8a0127e17b81e98464dd005c4fc1f8cada0
SPACEMIT_K1_FSBL_SITE = https://gitee.com/spacemit-buildroot/uboot-2022.10.git
SPACEMIT_K1_FSBL_SITE_METHOD = git
SPACEMIT_K1_FSBL_LICENSE = GPL-2.0+
SPACEMIT_K1_FSBL_LICENSE_FILES = Licenses/gpl-2.0.txt
SPACEMIT_K1_FSBL_INSTALL_TARGET = NO
SPACEMIT_K1_FSBL_INSTALL_IMAGES = YES
SPACEMIT_K1_FSBL_DEPENDENCIES = $(BR2_BISON_HOST_DEPENDENCY) \
	$(BR2_FLEX_HOST_DEPENDENCY) host-dtc host-openssl host-python3 opensbi

SPACEMIT_K1_FSBL_MAKE_OPTS = \
	CROSS_COMPILE="$(TARGET_CROSS)" \
	ARCH=riscv \
	HOSTCC="$(HOSTCC) $(subst -I/,-isystem /,$(subst -I /,-isystem /,$(HOST_CFLAGS)))" \
	HOSTLDFLAGS="$(HOST_LDFLAGS)" \
	DTC=$(HOST_DIR)/bin/dtc

# Building the SPL also signs it into FSBL.bin.  The OpenSBI FIT is
# made with the vendor mkimage: the FSBL checks a crc32 hash, and the
# mkimage from host-uboot-tools 2024.04 is built without crc32.
define SPACEMIT_K1_FSBL_BUILD_CMDS
	$(TARGET_MAKE_ENV) $(MAKE) -C $(@D) $(SPACEMIT_K1_FSBL_MAKE_OPTS) k1_defconfig
	$(TARGET_MAKE_ENV) $(MAKE) -C $(@D) $(SPACEMIT_K1_FSBL_MAKE_OPTS) spl/u-boot-spl
	cp $(SPACEMIT_K1_FSBL_PKGDIR)/fw_dynamic.its $(BINARIES_DIR)/fw_dynamic.bin $(@D)/
	$(TARGET_MAKE_ENV) $(@D)/tools/mkimage -f $(@D)/fw_dynamic.its $(@D)/fw_dynamic.itb
endef

define SPACEMIT_K1_FSBL_INSTALL_IMAGES_CMDS
	$(INSTALL) -D -m 0644 $(@D)/FSBL.bin $(BINARIES_DIR)/FSBL.bin
	$(INSTALL) -D -m 0644 $(@D)/bootinfo_sd.bin $(BINARIES_DIR)/bootinfo_sd.bin
	$(INSTALL) -D -m 0644 $(@D)/fw_dynamic.itb $(BINARIES_DIR)/fw_dynamic.itb
endef

$(eval $(generic-package))
