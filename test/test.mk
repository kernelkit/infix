test-dir := $(BR2_EXTERNAL_INFIX_PATH)/test

UNIT_TESTS ?= $(test-dir)/case/all-repo.yaml $(test-dir)/case/all-unit.yaml

test-unit:
	$(test-dir)/env $(test-dir)/9pm/9pm.py $(UNIT_TESTS)
