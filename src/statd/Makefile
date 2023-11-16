CFLAGS   += -Wall -Wextra -Werror
CPPFLAGS += -D_GNU_SOURCE
LDLIBS   += -lsysrepo -lev -lyang -ljansson -lite -lsrx

TARGET    = statd
SRC       = statd.c shared.c shared.h

all: $(TARGET)

$(TARGET): $(SRC)
	$(CC) $(CPPFLAGS) $(LDLIBS) $(CFLAGS) -o $(TARGET) $(SRC)

clean:
	-rm statd

distclean: clean
	-rm *~

install:
	install -D statd $(DESTDIR)/sbin/
