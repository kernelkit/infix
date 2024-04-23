test-dir := $(BR2_EXTERNAL_INFIX_PATH)/test
INFIX_TESTS ?= $(test-dir)/case/all.yaml

test-env = $(test-dir)/env \
	-f $(BINARIES_DIR)/infix-x86_64.img \
	-f $(BINARIES_DIR)/infix-x86_64-disk.img \
	-f $(BINARIES_DIR)/OVMF.fd \
        -p $(BINARIES_DIR)/infix-x86_64.pkg \
	$(1) $(2)

test-env-qeneth = $(call test-env,-q $(test-dir)/virt/quad,$(1))
test-env-run    = $(call test-env,-C -t $(BINARIES_DIR)/qemu.dot,$(1))

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

.PHONY: test test-sh test-qeneth test-qeneth-sh test-run test-run-sh test-run-play
