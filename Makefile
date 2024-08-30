export BR2_EXTERNAL ?= $(CURDIR)
export PATH         := $(CURDIR)/bin:$(PATH)

ARCH ?= $(shell uname -m)
O    ?= output

# If a relative output path is specified, we have to translate it to
# an absolute one before handing over control to Buildroot, which will
# otherwise treat it as relative to ./buildroot.
override O := $(if $(filter /%,$O),$O,$(CURDIR)/$O)

config := $(O)/.config
bmake   = $(MAKE) -C buildroot O=$(O) $1


all: $(config) buildroot/Makefile
	@+$(call bmake,$@)

check dep:
	@echo "Starting local check, stage $@ ..."
	@make -C src $@

$(config):
	@+$(call bmake,list-defconfigs)
	@echo "\e[7mERROR: No configuration selected.\e[0m"
	@echo "Please choose a configuration from the list above by running"
	@echo "'make <board>_defconfig' before building an image."
	@exit 1

%: | buildroot/Makefile
	@+$(call bmake,$@)

legal-info: | buildroot/Makefile
	$(call bmake,legal-info LINUX_LICENSE_FILES=COPYING)

# Workaround, see board/x86_64/board.mk
test:
	@+$(call bmake,$@)

buildroot/Makefile:
	@git submodule update --init

.PHONY: all check test
