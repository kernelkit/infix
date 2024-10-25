base-dir           := $(lastword $(subst :, ,$(BR2_EXTERNAL)))
test-dir           := $(BR2_EXTERNAL_INFIX_PATH)/test
ninepm             := $(BR2_EXTERNAL_INFIX_PATH)/test/9pm/9pm.py
spec-dir           := $(test-dir)/spec
test-specification := $(O)/images/test-specification.pdf

UNIT_TESTS         ?= $(test-dir)/case/all-repo.yaml $(test-dir)/case/all-unit.yaml
TESTS              ?= $(test-dir)/case/all.yaml
ifeq ($INFIX_OEM_PATH), "")
GIT_PATH            = $(INFIX_OEM_PATH)
else
GIT_PATH            = $(BR2_EXTERNAL_INFIX_PATH)
endif
GIT_VERSION         = $(shell git -C $(GIT_PATH) describe --dirty --always --tags)

base := -b $(base-dir)

TEST_MODE ?= qeneth
mode-qeneth := -q $(test-dir)/virt/quad
mode-host   := -t $(or $(TOPOLOGY),/etc/infamy.dot)
mode-run    := -t $(BINARIES_DIR)/qemu.dot
mode        := $(mode-$(TEST_MODE))

INFIX_IMAGE_ID := $(call qstrip,$(INFIX_IMAGE_ID))
binaries-$(ARCH) := $(addprefix $(INFIX_IMAGE_ID),.img -disk.img .pkg)
binaries-x86_64  += OVMF.fd
binaries := $(foreach bin,$(binaries-$(ARCH)),-f $(BINARIES_DIR)/$(bin))

# Common transport override for minimal defconfigs
ifneq ($(BR2_PACKAGE_ROUSETTE),y)
export INFAMY_ARGS := --transport=netconf
endif

test:
	$(test-dir)/env -r $(base) $(mode) $(binaries) $(ninepm) $(TESTS)

test-sh:
	$(test-dir)/env $(base) $(mode) $(binaries) -i /bin/sh

test-spec:
	@sed 's/{REPLACE}/$(subst ",,$(INFIX_NAME)) $(GIT_VERSION)/'  $(spec-dir)/Readme.adoc.in > $(spec-dir)/Readme.adoc
	@$(spec-dir)/generate_spec.py -d $(test-dir)/case -r $(BR2_EXTERNAL_INFIX_PATH)
	@asciidoctor-pdf --failure-level INFO --theme $(spec-dir)/theme.yml -a pdf-fontsdir=$(spec-dir)/fonts -o $(test-specification) $(spec-dir)/Readme.adoc

# Unit tests run with random (-r) hostname and container name to
# prevent race conditions when running in CI environments.
test-unit:
	$(test-dir)/env -r $(base) $(ninepm) $(UNIT_TESTS)

.PHONY: test test-sh test-unit test-spec
