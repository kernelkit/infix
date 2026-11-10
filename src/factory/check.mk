# Used by ../Makefile for 'make check' and Coverity Scan

all:
	make all CPPFLAGS=-DRESETME_PATH=/mnt/cfg/infix.reset
	make clean
