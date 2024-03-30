# Host build of critical components, for Coverity Scan mostly
APPS = confd execd factory keyack statd

.PHONY: all
all:
	@echo "*** all not supported, only check possible ***"
	false

.PHONY: dep
dep:
	(cd libsrx && make -f check.mk dep)

check: dep $(APPS)
	rm -rf staging

$(APPS): libsrx
	(cd $@ && make -f check.mk)

.PHONY: libsrx
libsrx:
	(cd $@ && make -f check.mk)
