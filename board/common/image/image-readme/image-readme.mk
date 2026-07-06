################################################################################
#
# image-readme
#
################################################################################

IMAGE_README_DIR := $(pkgdir)

image-readme: $(BINARIES_DIR)/README.md

$(BINARIES_DIR)/README.md: $(IMAGE_README_DIR)/README.md
	@$(call IXMSG,"Installing README.md")
	@mkdir -p $(BINARIES_DIR)
	@cp $< $@

ifeq ($(IX_IMAGE_README),y)
TARGETS_ROOTFS += image-readme
endif
