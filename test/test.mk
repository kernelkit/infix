base-dir           := $(lastword $(subst :, ,$(BR2_EXTERNAL)))
test-dir           ?= $(BR2_EXTERNAL_INFIX_PATH)/test
ninepm             := $(BR2_EXTERNAL_INFIX_PATH)/test/9pm/9pm.py
ninepm_report      := $(BR2_EXTERNAL_INFIX_PATH)/test/9pm/report.py
NINEPM_PROJ_CONF   ?= $(BR2_EXTERNAL_INFIX_PATH)/test/9pm-proj.yaml
spec-dir           := $(test-dir)/spec
test-specification := $(BINARIES_DIR)/test-specification.pdf
test-report        := $(BINARIES_DIR)/test-report.pdf
LOGO               ?= $(test-dir)/../doc/logo.png[top=40%, align=right, pdfwidth=10cm]
UNIT_TESTS         ?= $(test-dir)/case/all-repo.yaml $(test-dir)/case/all-unit.yaml
TESTS              ?= $(test-dir)/case/all.yaml

yang_extractor     := $(test-dir)/utils/extract_xpaths.py
coverage_reporter  := $(test-dir)/utils/coverage_report.py
YANG_DIR           ?= $(BR2_EXTERNAL_INFIX_PATH)/src/confd/yang/confd
xpaths_all_csv     := $(test-dir)/.log/xpaths_all.csv

base := -b $(base-dir)

TEST_MODE ?= qeneth
mode-qeneth := -q $(or $(QTOPOLOGY),$(test-dir)/virt/quad)
mode-host   := -t $(or $(TOPOLOGY),/etc/infamy.dot)
mode-run    := -t $(BINARIES_DIR)/qemu.dot
mode        := $(mode-$(TEST_MODE))

pkg-$(ARCH)      := -p $(O)/images/$(INFIX_ARTIFACT).pkg
binaries-$(ARCH) := $(INFIX_ARTIFACT).qcow2
binaries-x86_64  += OVMF.fd
binaries         := $(foreach bin,$(binaries-$(ARCH)),-f $(BINARIES_DIR)/$(bin))

# Common transport override for minimal defconfigs
ifneq ($(BR2_PACKAGE_ROUSETTE),y)
export INFAMY_ARGS := --transport=netconf
endif

test:
	$(test-dir)/env -r $(base) $(mode) $(binaries) $(pkg-$(ARCH)) \
		sh -c 'test -f $(xpaths_all_csv) || python3 $(yang_extractor) $(YANG_DIR) $(xpaths_all_csv) || true; \
		       $(ninepm) -v $(TESTS); rc=$$?; \
		       $(ninepm_report) github   $(test-dir)/.log/last/result.json; \
		       $(ninepm_report) asciidoc $(test-dir)/.log/last/result.json; \
		       python3 $(coverage_reporter) \
		           $(xpaths_all_csv) \
		           $(test-dir)/.log/last/xpath_coverage.log \
		           $(test-dir)/.log/last/xpath_coverage_report.md \
		           2>/dev/null || true; \
		       chmod -R 777 $(test-dir)/.log; \
		       exit $$rc'

test-sh:
	$(test-dir)/env $(base) $(mode) $(binaries) $(pkg-$(ARCH)) -i /bin/sh

SPEC_DEBUG :=
SPEC_Q := @
ifeq ($(V),1)
SPEC_DEBUG := -d
SPEC_Q :=
endif

test-spec:
	@esc_infix_name="$(echo $(INFIX_NAME) | sed 's/\//\\\//g')"; \
	sed 's/{REPLACE}/$(subst ",,$(esc_infix_name)) $(INFIX_VERSION)/' \
		$(spec-dir)/Readme.adoc.in > $(spec-dir)/Readme.adoc
	$(SPEC_Q)$(spec-dir)/generate_spec.py -s $(test-dir)/case/all.yaml -r $(BR2_EXTERNAL_INFIX_PATH) $(SPEC_DEBUG)
	@asciidoctor-pdf --failure-level INFO --theme $(spec-dir)/theme.yml \
		-a logo="image:$(LOGO)" \
	 	-a pdf-fontsdir=$(spec-dir)/fonts \
	 	-o $(test-specification) $(spec-dir)/Readme.adoc

test-report:
	$(ninepm_report) pdf $(test-dir)/.log/last/result.json \
		--theme $(spec-dir)/theme.yml \
		--fontsdir $(spec-dir)/fonts \
		--logo "$(LOGO)" \
		-o $(test-report)

xpath_coverage_report_md   := $(test-dir)/.log/last/xpath_coverage_report.md
xpath_coverage_report_css  := $(test-dir)/utils/coverage_report.css
xpath_coverage_report_pdf  := $(BINARIES_DIR)/xpath-coverage-report.pdf
xpath_coverage_renderer    := $(test-dir)/utils/render_coverage_pdf.sh
xpath_coverage_logo        := $(abspath $(test-dir)/../doc/logo.png)
# Rendered inside the infix-test container, which carries pandoc, weasyprint
# and the fonts; make variables are expanded on the host and passed as args.
xpath-coverage-report:
	$(test-dir)/env $(base) $(xpath_coverage_renderer) \
		$(xpath_coverage_report_md) \
		$(xpath_coverage_report_css) \
		$(xpath_coverage_logo) \
		"$(subst ",,$(INFIX_NAME)) $(INFIX_VERSION)" \
		$(xpath_coverage_report_pdf)

# Unit tests run with random (-r) hostname and container name to
# prevent race conditions when running in CI environments.
test-unit:
	$(test-dir)/env -r $(base) $(ninepm) -v $(UNIT_TESTS)

.PHONY: test test-sh test-unit test-spec xpath-coverage-report
