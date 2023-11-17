# Used by ../Makefile for 'make check' and Coverity Scan
export PKG_CONFIG_PATH = $(CURDIR)/../staging/lib/pkgconfig

all:
	./autogen.sh
	./configure
	make all
	make distclean
