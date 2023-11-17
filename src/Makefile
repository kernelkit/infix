# Host build of critical components, for Coverity Scan mostly
APPS = confd statd factory

all:
	@echo "*** all not supported, only check possible ***"
	false

dep: libsrx
	(cd $< && make -f check.mk $@)

check: $(APPS)
	rm -rf staging

$(APPS): libsrx
	(cd $@ && make -f check.mk)

.PHONY: libsrx
libsrx:
	(cd $@ && make -f check.mk)
