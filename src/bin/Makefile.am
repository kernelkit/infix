DISTCLEANFILES   = *~ *.d
ACLOCAL_AMFLAGS  = -I m4

bin_PROGRAMS     = copy erase files

copy_SOURCES     = copy.c util.c util.h
copy_CPPFLAGS    = -D_DEFAULT_SOURCE -D_GNU_SOURCE
copy_CFLAGS      = -W -Wall -Wextra
copy_CFLAGS     += $(libite_CFLAGS) $(sysrepo_CFLAGS)
copy_LDADD       = $(libite_LIBS)   $(sysrepo_LIBS)

erase_SOURCES    = erase.c util.c util.h
erase_CPPFLAGS   = -D_DEFAULT_SOURCE -D_GNU_SOURCE
erase_CFLAGS     = -W -Wall -Wextra
erase_CFLAGS    += $(libite_CFLAGS) $(sysrepo_CFLAGS)
erase_LDADD      = $(libite_LIBS)   $(sysrepo_LIBS)

files_SOURCES    = files.c util.c util.h
files_CPPFLAGS   = -D_DEFAULT_SOURCE -D_GNU_SOURCE
files_CFLAGS     = -W -Wall -Wextra
files_CFLAGS    += $(libite_CFLAGS) $(sysrepo_CFLAGS)
files_LDADD      = $(libite_LIBS)   $(sysrepo_LIBS)
