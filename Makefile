export BR2_EXTERNAL ?= $(CURDIR)
export PATH         := $(CURDIR)/bin:$(PATH)

ARCH ?= $(shell uname -m)
O    ?= output

# If a relative output path is specified, we have to translate it to
# an absolute one before handing over control to Buildroot, which will
# otherwise treat it as relative to ./buildroot.
override O := $(if $(filter /%,$O),$O,$(CURDIR)/$O)

config      := $(O)/.config
bmake        = $(MAKE) -C buildroot O=$(O) $1
SNIPPETS_DIR := $(CURDIR)/configs/snippets
MERGE_CONFIG := $(CURDIR)/buildroot/support/kconfig/merge_config.sh


all: $(config) buildroot/Makefile
	@+$(call bmake,$@)

check dep coverity:
	@make -C src $@

$(config):
	@+$(call bmake,list-defconfigs)
	@echo "\e[7mERROR: No configuration selected.\e[0m"
	@echo "Please choose a configuration from the list above by running"
	@echo "'make <board>_defconfig' before building an image."
	@exit 1

apply-%: $(SNIPPETS_DIR)/%.conf | $(config)
	@KCONFIG_CONFIG=$(config) $(MERGE_CONFIG) -m $(config) $<
	@+$(call bmake,olddefconfig)
	@echo "Applied snippet: $<"

list-snippets:
	@echo "Available snippets (use 'make apply-<name>'):"
	@ls $(SNIPPETS_DIR)/*.conf 2>/dev/null | sed 's|.*/||; s|\.conf$$||; s|^|  |'

dev: | $(config)
	@for s in $(SNIPPETS_DIR)/*.conf; do \
		KCONFIG_CONFIG=$(config) $(MERGE_CONFIG) -m $(config) $$s; \
		echo "Applied snippet: $$s"; \
	done
	@+$(call bmake,olddefconfig)
	@+$(call bmake,all)

%: | buildroot/Makefile
	@+$(call bmake,$@)

legal-info: | buildroot/Makefile
	$(call bmake,legal-info LINUX_LICENSE_FILES=COPYING)

cyclonedx: | buildroot/Makefile
	@echo "Generating package information..."
	@$(MAKE) --no-print-directory -C buildroot O=$(O) show-info | ./buildroot/utils/generate-cyclonedx > $(O)/cyclonedx-sbom.json
	@echo "CycloneDX SBOM generated: $(O)/cyclonedx-sbom.json"

# Workaround, see board/x86_64/board.mk
test:
	@+$(call bmake,$@)

buildroot/Makefile:
	@git submodule update --init

.PHONY: all check coverity dep test cyclonedx list-snippets dev
