test-dir := $(BR2_EXTERNAL_INFIX_PATH)/test

UNIT_TESTS  ?= $(test-dir)/case/all-repo.yaml $(test-dir)/case/all-unit.yaml
INFIX_TESTS ?= $(test-dir)/case/all.yaml

binaries-$(ARCH) := $(addprefix infix-$(ARCH),.img -disk.img .pkg)
binaries-x86_64  += OVMF.fd
binaries := $(foreach bin,$(binaries-$(ARCH)),-f $(BINARIES_DIR)/$(bin))

test-env-qeneth = $(test-dir)/env $(binaries) -q $(test-dir)/virt/quad $(1)
test-env-run    = $(test-dir)/env $(binaries) -C -t $(BINARIES_DIR)/qemu.dot $(1)

test test-qeneth:
	$(call test-env-qeneth,\
		$(BR2_EXTERNAL_INFIX_PATH)/test/9pm/9pm.py \
			$(INFIX_TESTS))
test-sh test-qeneth-sh:
	$(call test-env-qeneth,-i /bin/sh)

test-run: | ~/.infix-test-venv
	$(call test-env-run,\
		$(BR2_EXTERNAL_INFIX_PATH)/test/9pm/9pm.py \
			$(INFIX_TESTS))
test-run-sh: | ~/.infix-test-venv
	$(call test-env-run,/bin/sh)

test-run-play: | ~/.infix-test-venv
	$(call test-env-run,$(test-dir)/case/meta/play.py)

~/.infix-test-venv:
	$(test-dir)/docker/init-venv.sh $(test-dir)/docker/pip-requirements.txt

test-unit:
	$(test-dir)/env $(test-dir)/9pm/9pm.py $(UNIT_TESTS)

.PHONY: test test-sh test-qeneth test-qeneth-sh test-run test-run-sh test-run-play test-unit
