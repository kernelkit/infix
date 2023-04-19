EXTRA_DIST         = lib.sh $(TESTS)
pkgdata_DATA       = $(EXTRA_DIST)

TEST_EXTENSIONS    = .sh
TESTS_ENVIRONMENT  = unshare -mrun
TESTS              = three-independent.sh
TESTS 		  += bridge.sh
TESTS 		  += bridge-lag.sh

CLEANFILES         = *~ *.trs *.log
