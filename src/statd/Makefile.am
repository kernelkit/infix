DISTCLEANFILES      = *~ *.d
ACLOCAL_AMFLAGS     = -I m4

sbin_PROGRAMS       = statd
statd_SOURCES       = statd.c shared.c shared.h journal.c journal_retention.c journal.h
statd_CPPFLAGS      = -D_DEFAULT_SOURCE -D_GNU_SOURCE
statd_CFLAGS        = -W -Wall -Wextra
statd_CFLAGS       += $(jansson_CFLAGS) $(libyang_CFLAGS) $(sysrepo_CFLAGS)
statd_CFLAGS       += $(libsrx_CFLAGS) $(libite_CFLAGS)
statd_LDADD         = $(jansson_LIBS) $(libyang_LIBS) $(sysrepo_LIBS)
statd_LDADD        += $(libsrx_LIBS) $(libite_LIBS) $(EV_LIBS)

# Test stub for journal retention policy (no dependencies, standalone)
noinst_PROGRAMS     = journal_retention_stub
journal_retention_stub_SOURCES  = journal_retention_stub.c journal_retention.c journal.h
journal_retention_stub_CPPFLAGS = -D_DEFAULT_SOURCE -D_GNU_SOURCE -DJOURNAL_RETENTION_STUB
journal_retention_stub_CFLAGS   = -W -Wall -Wextra
journal_retention_stub_LDFLAGS  = -static
