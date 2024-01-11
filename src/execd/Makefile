CFLAGS   += -Wall -Wextra -Werror -Wno-unused
CPPFLAGS += -D_GNU_SOURCE
LDLIBS   += -luev -lite

TARGET    = execd
SRC       = execd.c
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
