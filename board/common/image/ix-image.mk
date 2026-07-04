define inner-ix-image

$(2)_DIR := $$(pkgdir)

$(1): $$($(2)_DEPENDENCIES)
	@$$(call IXMSG,"$$(if $$($(2)_MESSAGE),$$($(2)_MESSAGE),Creating $(1))")
	@mkdir -p $$(BUILD_DIR)/$(1)
	@ \
	PATH=$$(BR_PATH) \
	PKGDIR=$$($(2)_DIR) \
	WORKDIR=$$(BUILD_DIR)/$(1) \
	BINARIES_DIR=$$(BINARIES_DIR) \
	BR2_EXTERNAL_INFIX_PATH=$$(BR2_EXTERNAL_INFIX_PATH) \
	ARTIFACT=$$(INFIX_ARTIFACT) \
	COMPATIBLE=$$(IX_COMPATIBLE) \
	VERSION=$$(INFIX_VERSION) \
	$$(foreach var,$$($(2)_CONFIG_VARS),$$(var)=$$($(3)_$$(var)) ) \
	$$($(2)_DIR)/generate.sh $$($(2)_OPTS)

ifeq ($$($(3)),y)
TARGETS_ROOTFS += $(1)
endif

endef

# $(2) is the package-local make-var prefix (UPPERCASE pkgname); $(3) is
# the IX_ prefixed Kconfig symbol name, see issue #1305.
ix-image = $(call inner-ix-image,$(pkgname),$(call UPPERCASE,$(pkgname)),IX_$(call UPPERCASE,$(pkgname)))
