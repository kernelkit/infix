all:
	./autogen.sh
	./configure
	make all
	make distclean
