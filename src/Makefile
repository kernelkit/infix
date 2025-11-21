# Host build of critical components, for static analysis
#
# Available targets:
#   build        - Build all components (no static analysis)
#   check        - Run static analysis (auto-detects scan-build or cppcheck)
#   scan-build   - Run clang static analyzer (preferred)
#   cppcheck     - Run cppcheck static analyzer (fallback)
#   coverity     - Show Coverity Scan usage
#   coverity-build - Build for Coverity Scan (used by CI)
#   dep          - Build dependencies only
#
APPS = bin confd factory keyack statd

# Detect available static analysis tools
HAVE_SCANBUILD := $(shell command -v scan-build 2>/dev/null)
HAVE_CPPCHECK  := $(shell command -v cppcheck 2>/dev/null)

.PHONY: all
all:
	@echo "*** all not supported, only build/check/coverity possible ***"
	@false

.PHONY: dep
dep:
	(cd libsrx && make -f check.mk dep)

# Main build target (renamed from check)
.PHONY: build
build: dep $(APPS)
	rm -rf staging

$(APPS): libsrx
	(cd $@ && make -f check.mk)

.PHONY: libsrx
libsrx:
	(cd $@ && make -f check.mk)

# Static analysis target - auto-detects scan-build or cppcheck
.PHONY: check
check:
ifdef HAVE_SCANBUILD
	@echo "==> Running scan-build (clang static analyzer)"
	$(MAKE) scan-build
else ifdef HAVE_CPPCHECK
	@echo "==> Running cppcheck (fallback - scan-build not found)"
	@echo "    For better analysis, install scan-build:"
	@echo "      Debian/Ubuntu: apt-get install clang-tools"
	@echo "      Fedora/RHEL:   dnf install clang-tools-extra"
	@echo ""
	$(MAKE) cppcheck
else
	@echo "*** ERROR: No static analysis tool found ***"
	@echo ""
	@echo "Please install scan-build (recommended) or cppcheck:"
	@echo "  Debian/Ubuntu: sudo apt-get install clang-tools"
	@echo "  Fedora/RHEL:   sudo dnf install clang-tools-extra"
	@echo "  Alpine:        apk add clang-extra-tools"
	@echo ""
	@echo "Or install cppcheck as fallback:"
	@echo "  Debian/Ubuntu: sudo apt-get install cppcheck"
	@echo "  Fedora/RHEL:   sudo dnf install cppcheck"
	@false
endif

# Clang static analyzer (preferred)
.PHONY: scan-build
scan-build: dep
ifndef HAVE_SCANBUILD
	@echo "*** ERROR: scan-build not found ***"
	@echo ""
	@echo "Install scan-build for better static analysis:"
	@echo "  Debian/Ubuntu: sudo apt-get install clang-tools"
	@echo "  Fedora/RHEL:   sudo dnf install clang-tools-extra"
	@echo "  Alpine:        apk add clang-extra-tools"
	@false
endif
	@echo "==> Running scan-build on all components"
	@rm -rf scan-results
	scan-build -o scan-results --status-bugs $(MAKE) _analyze
	@echo "==> Scan complete. Results in scan-results/"

# cppcheck static analyzer
.PHONY: cppcheck
cppcheck: dep
ifndef HAVE_CPPCHECK
	@echo "*** ERROR: cppcheck not found ***"
	@echo ""
	@echo "Install cppcheck:"
	@echo "  Debian/Ubuntu: sudo apt-get install cppcheck"
	@echo "  Fedora/RHEL:   sudo dnf install cppcheck"
	@echo ""
	@echo "Or use scan-build (recommended) instead:"
	@echo "  Debian/Ubuntu: sudo apt-get install clang-tools"
	@false
endif
	@echo "==> Running cppcheck on all components"
	@for app in libsrx $(APPS); do \
		echo "  -> Checking $$app"; \
		(cd $$app && cppcheck --enable=all --suppress=missingIncludeSystem \
			--quiet --template=gcc -I../staging/include . 2>&1) || true; \
	done
	@echo "==> cppcheck complete"

# Internal target for scan-build to analyze
.PHONY: _analyze
_analyze: libsrx $(APPS)
	rm -rf staging

# Coverity Scan target (for CI)
.PHONY: coverity
coverity:
	@echo "==> Building for Coverity Scan"
	@echo "Use: cov-build --dir cov-int make coverity-build"

.PHONY: coverity-build
coverity-build: build
