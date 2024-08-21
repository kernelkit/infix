test-dir := $(BR2_EXTERNAL_INFIX_PATH)/test
ninepm   := $(BR2_EXTERNAL_INFIX_PATH)/test/9pm/9pm.py

UNIT_TESTS  ?= $(test-dir)/case/all-repo.yaml $(test-dir)/case/all-unit.yaml
INFIX_TESTS ?= $(test-dir)/case/all.yaml

TEST_MODE ?= qeneth
mode-qeneth := -q $(test-dir)/virt/quad
mode-host   := -t $(or $(TOPOLOGY),/etc/infamy.dot)
mode-run    := -t $(BINARIES_DIR)/qemu.dot
mode        := $(mode-$(TEST_MODE))

binaries-$(ARCH) := $(addprefix infix-$(ARCH),.img -disk.img .pkg)
binaries-x86_64  += OVMF.fd
binaries := $(foreach bin,$(binaries-$(ARCH)),-f $(BINARIES_DIR)/$(bin))

test:
	$(test-dir)/env -r $(mode) $(binaries) $(ninepm) $(INFIX_TESTS)

test-sh:
	$(test-dir)/env $(mode) $(binaries) -i /bin/sh

# Unit tests run with random (-r) hostname and container name to
# prevent race conditions when running in CI environments.
test-unit:
	$(test-dir)/env -r $(ninepm) $(UNIT_TESTS)

.PHONY: test test-sh test-unit
