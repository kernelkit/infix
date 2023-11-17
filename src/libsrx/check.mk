# Used by ../Makefile for 'make check' and Coverity Scan

all:
	./configure
	make all
	make distclean

# Normal build, for apps to link to during check
dep:
	./autogen.sh
	./configure --prefix=$(CURDIR)/../staging
	make all
	make install
	make distclean
