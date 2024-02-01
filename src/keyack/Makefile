CFLAGS += -Wall -Wextra -Werror

all: keyack

clean:
	-rm -f keyack

distclean: clean
	-rm *~

install:
	install -D keyack $(DESTDIR)/sbin/
