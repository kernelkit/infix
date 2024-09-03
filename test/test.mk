base-dir           := $(lastword $(subst :, ,$(BR2_EXTERNAL)))
test-dir           := $(BR2_EXTERNAL_INFIX_PATH)/test
ninepm             := $(BR2_EXTERNAL_INFIX_PATH)/test/9pm/9pm.py
spec-dir           := $(test-dir)/spec
test-specification := $(O)/images/test-specification.pdf

UNIT_TESTS  ?= $(test-dir)/case/all-repo.yaml $(test-dir)/case/all-unit.yaml
TESTS ?= $(test-dir)/case/all.yaml

IMAGE ?= infix
TOPOLOGY-DIR ?= $(test-dir)/virt/quad

base := -b $(base-dir)

TEST_MODE ?= qeneth
mode-qeneth := -q $(TOPOLOGY-DIR)
mode-host   := -t $(or $(TOPOLOGY),/etc/infamy.dot)
mode-run    := -t $(BINARIES_DIR)/qemu.dot
mode        := $(mode-$(TEST_MODE))

binaries-$(ARCH) := $(addprefix $(IMAGE)-$(ARCH),.img -disk.img .pkg)
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
	@sed 's/{REPLACE}/$(subst ",,$(INFIX_NAME))/'  $(spec-dir)/Readme.adoc.in > $(spec-dir)/Readme.adoc
	@$(spec-dir)/generate_spec.py -d $(test-dir)/case
	@asciidoctor-pdf --theme $(spec-dir)/theme.yml -a pdf-fontsdir=$(spec-dir)/fonts -o $(test-specification) $(spec-dir)/Readme.adoc

# Unit tests run with random (-r) hostname and container name to
# prevent race conditions when running in CI environments.
test-unit:
	$(test-dir)/env -r $(base) $(ninepm) $(UNIT_TESTS)

.PHONY: test test-sh test-unit test-spec
