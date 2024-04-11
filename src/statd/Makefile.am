DISTCLEANFILES      = *~ *.d
ACLOCAL_AMFLAGS     = -I m4

sbin_PROGRAMS       = statd
statd_SOURCES       = statd.c shared.c shared.h
statd_CPPFLAGS      = -D_DEFAULT_SOURCE -D_GNU_SOURCE
statd_CFLAGS        = -W -Wall -Wextra
statd_CFLAGS       += $(jansson_CFLAGS) $(libyang_CFLAGS) $(sysrepo_CFLAGS)
statd_CFLAGS       += $(libsrx_CFLAGS) $(libite_CFLAGS)
statd_LDADD         = $(jansson_LIBS) $(libyang_LIBS) $(sysrepo_LIBS)
statd_LDADD        += $(libsrx_LIBS) $(libite_LIBS)
