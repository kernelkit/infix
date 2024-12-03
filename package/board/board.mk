define inner-ix-board

$(2)_VERSION = ix-board
$(2)_LICENSE = BSD-3-Clause
$(2)_LICENSE_FILES = LICENSE
$(2)_SITE_METHOD = local
$(2)_SITE = $$(BR2_EXTERNAL_INFIX_PATH)/src/board/$(1)
$(2)_REDISTRIBUTE = NO

# The kernel must be built first.
$(2)_DEPENDENCIES += \
	linux \
	$$(BR2_MAKE_HOST_DEPENDENCY)

# This is only defined in some infrastructures (e.g. autotools, cmake),
# but not in others (e.g. generic). So define it here as well.
$(2)_MAKE ?= $$(BR2_MAKE)

define $(2)_DTBS_BUILD
	@$$(call MESSAGE,"Building device tree blob(s)")
	$$(LINUX_MAKE_ENV) $$($$(PKG)_MAKE) \
		-C $$(LINUX_DIR) \
		$$(LINUX_MAKE_FLAGS) \
		$$($(2)_DTB_MAKE_OPTS) \
		PWD=$$(@D)/dts \
		M=$$(@D)/dts \
		modules
endef
$(2)_POST_BUILD_HOOKS += $(2)_DTBS_BUILD

define $(2)_DTBS_INSTALL_TARGET
	@$$(call MESSAGE,"Installing device tree blob(s)")
	$$(TARGET_MAKE_ENV) $$(TARGET_CONFIGURE_OPTS) $$($$(PKG)_MAKE) \
		-f $$(BR2_EXTERNAL_INFIX_PATH)/package/board/dtb-inst.makefile \
		-C $$(@D)/dts \
		DESTDIR="$$(TARGET_DIR)" \
		install
endef
$(2)_POST_INSTALL_TARGET_HOOKS += $(2)_DTBS_INSTALL_TARGET

endef

ix-board = $(call inner-ix-board,$(pkgname),$(call UPPERCASE,$(pkgname)))

include $(sort $(wildcard $(BR2_EXTERNAL_INFIX_PATH)/package/board/*/*.mk))
