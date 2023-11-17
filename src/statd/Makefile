CFLAGS   += -Wall -Wextra -Werror
CPPFLAGS += -D_GNU_SOURCE
LDLIBS   += -lsysrepo -lev -lyang -ljansson -lsrx -lite

TARGET    = statd
HEADERS   = shared.h
SRC       = statd.c shared.c
OBJ       = $(SRC:.c=.o)

all: $(TARGET)

.c.o:
	$(CC) $(CFLAGS) $(CPPFLAGS) -c -o $@ $<

$(OBJ): $(SRC) $(HEADERS)

$(TARGET): $(OBJ)
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ $^ $(LDLIBS)

clean:
	-rm -f $(TARGET) $(OBJ)

distclean: clean
	-rm *~

install: $(TARGET)
	install -D $(TARGET) $(DESTDIR)/sbin/
