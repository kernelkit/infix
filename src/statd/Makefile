# TODO: Libs are included fron confd. They should be broken out of confd and
# shared between statd and confd in some nice way.
CONFD_LIB = $(BASE_DIR)/../src/confd/lib/
CONFD_SRC_LIB = $(BASE_DIR)/../src/confd/src/lib

CFLAGS += -Wall -Wextra -Werror
LDLIBS += -lsysrepo -lev -lyang -ljansson -lite
CPPFLAGS := -I$(CONFD_SRC_LIB)

TARGET = statd
SRC = statd.c
LIB_SRC = $(CONFD_SRC_LIB)/helpers.c $(CONFD_SRC_LIB)/lyx.c $(CONFD_LIB)/vasprintf.c
LIB_HDR = $(CONFD_SRC_LIB)/helpers.h $(CONFD_SRC_LIB)/lyx.h

all: $(TARGET)

$(TARGET): $(SRC) $(LIB_SRC) $(LIB_HDR)
	$(CC) $(CPPFLAGS) $(LDLIBS) $(CFLAGS) -o $(TARGET) $(SRC) $(LIB_SRC)

clean:
	-rm statd

distclean: clean
	-rm *~

install:
	install -D statd $(DESTDIR)/sbin/
