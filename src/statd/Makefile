CFLAGS   += -Wall -Wextra -Werror
LDLIBS   += -lsysrepo -lev -lyang -ljansson -lite -lsrx

TARGET    = statd
SRC       = statd.c shared.c shared.h \
	    iface-ip-link.c iface-ip-link.h \
	    iface-ip-addr.c iface-ip-addr.h \
	    iface-ethtool.c iface-ethtool.h

all: $(TARGET)

$(TARGET): $(SRC)
	$(CC) $(CPPFLAGS) $(LDLIBS) $(CFLAGS) -o $(TARGET) $(SRC)

clean:
	-rm statd

distclean: clean
	-rm *~

install:
	install -D statd $(DESTDIR)/sbin/
