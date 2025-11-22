# Host build of critical components, for static analysis
#
# Available targets:
#   build          - Build all components (no static analysis)
#   check          - Run static analysis (auto-detects scan-build or cppcheck)
#   coverity       - Build for Coverity Scan (used by CI)
#   dep            - Build dependencies only
#
# Installation (for 'make check'):
#   scan-build (recommended):
#     Debian/Ubuntu: sudo apt-get install clang-tools
#     Fedora/RHEL:   sudo dnf install clang-tools-extra
#     Alpine:        sudo apk add clang-extra-tools
#
#   cppcheck (fallback):
#     Debian/Ubuntu: sudo apt-get install cppcheck
#     Fedora/RHEL:   sudo dnf install cppcheck
#
APPS            = bin confd factory keyack statd
HAVE_SCANBUILD := $(shell command -v scan-build 2>/dev/null)
HAVE_CPPCHECK  := $(shell command -v cppcheck 2>/dev/null)

all:
	@echo "Target 'all' not supported, use build/check/coverity instead"
	@false

dep:
	(cd libsrx && make -f check.mk dep)

build: dep $(APPS)
	rm -rf staging

$(APPS): libsrx
	(cd $@ && make -f check.mk)

libsrx:
	(cd $@ && make -f check.mk)

# Static analysis - auto-detects scan-build or cppcheck
check: dep
ifdef HAVE_SCANBUILD
	@rm -rf scan-results
	@scan-build -o scan-results --status-bugs $(MAKE) build
	@echo "Scan complete. Results in scan-results/"
else ifdef HAVE_CPPCHECK
	@for app in libsrx $(APPS); do							\
		(cd $$app && cppcheck --enable=all --suppress=missingIncludeSystem	\
			--quiet --template=gcc -I../staging/include . 2>&1) || true;	\
	done
else
	@echo "Error: No static analysis tool found."
	@false
endif

# Coverity Scan (for CI)
coverity: build

.PHONY: all dep build libsrx check coverity
