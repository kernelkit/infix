DISTCLEANFILES      = *~ *.d
ACLOCAL_AMFLAGS     = -I m4

sbin_PROGRAMS       = execd
execd_SOURCES       = execd.c execd.h
execd_CPPFLAGS      = -D_GNU_SOURCE
execd_CFLAGS        = -W -Wall -Wextra -Wno-unused
execd_CFLAGS        = $(libuev_CFLAGS) $(libite_CFLAGS)
execd_LDADD         = $(libuev_LIBS) $(libite_LIBS)
