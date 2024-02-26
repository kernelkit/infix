DISTCLEANFILES      = *~ *.d
ACLOCAL_AMFLAGS     = -I m4

pkglibexec_SCRIPTS  = cli-pretty yanger ospf-status

sbin_PROGRAMS       = statd
statd_SOURCES       = statd.c shared.c shared.h
statd_CPPFLAGS      = -D_DEFAULT_SOURCE -D_GNU_SOURCE
statd_CFLAGS        = -W -Wall -Wextra
statd_CFLAGS       += $(jansson_CFLAGS) $(libite_CFLAGS) $(libyang_CFLAGS)
statd_CFLAGS       += $(sysrepo_CFLAGS) $(libsrx_CFLAGS)
statd_LDADD         = $(jansson_LIBS) $(libite_LIBS) $(libyang_LIBS)
statd_LDADD        += $(sysrepo_LIBS) $(libsrx_LIBS)
