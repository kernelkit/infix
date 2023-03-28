################################################################################
#
# mech
#
################################################################################

MECH_VERSION = 1.0
MECH_LICENSE = Apache-2.0
MECH_SITE_METHOD = local
MECH_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/mech
MECH_DEPENDENCIES = augeas clixon
MECH_AUTORECONF = YES

define MECH_INSTALL_EXTRA
	cp $(MECH_PKGDIR)/clixon.conf   $(FINIT_D)/available/
	ln -sf ../available/clixon.conf $(FINIT_D)/enabled/clixon.conf
	cp $(MECH_PKGDIR)/tmpfiles.conf $(TARGET_DIR)/etc/tmpfiles.d/mech.conf
	mkdir -p $(TARGET_DIR)/lib/infix
	cp $(MECH_PKGDIR)/prep-db       $(TARGET_DIR)/lib/infix/
	cp $(MECH_PKGDIR)/clean-etc     $(TARGET_DIR)/lib/infix/
endef
MECH_TARGET_FINALIZE_HOOKS += MECH_INSTALL_EXTRA

define MECH_GEN_YANG_TREE
	if which pyang 2>/dev/null; then \
		pyang -f tree \
			-p $(TARGET_DIR)/usr/share/clixon \
			$$(find $(TARGET_DIR)/usr/share/clixon/ -name '*@*.yang') >$(BINARIES_DIR)/mech-yang.txt; \
		pyang -f jstree \
			-p $(TARGET_DIR)/usr/share/clixon \
			$$(find $(TARGET_DIR)/usr/share/clixon/ -name '*@*.yang') >$(BINARIES_DIR)/mech-yang.html; \
	fi
endef
MECH_POST_INSTALL_TARGET_HOOKS += MECH_GEN_YANG_TREE

$(eval $(autotools-package))
