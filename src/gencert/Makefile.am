DISTCLEANFILES      = *~ *.d
ACLOCAL_AMFLAGS     = -I m4

bin_PROGRAMS        = gencert
gencert_SOURCES     = gencert.c
gencert_CFLAGS      = -W -Wall -Wextra -Wno-unused
gencert_CFLAGS      = $(ssl_CFLAGS)
gencert_LDADD       = $(ssl_LIBS)
