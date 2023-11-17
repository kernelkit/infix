# Used by ../Makefile for 'make check' and Coverity Scan

export CPPFLAGS = -I$(CURDIR)/../staging/include
export LDLIBS   = -L$(CURDIR)/../staging/lib -L/usr/lib/x86_64-linux-gnu

all:
	make all
	make clean
