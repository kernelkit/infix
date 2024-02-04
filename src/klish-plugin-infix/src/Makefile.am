AM_CPPFLAGS  = -D_DEFAULT_SOURCE -D_XOPEN_SOURCE -D_GNU_SOURCE
CFLAGS       = -Wall -Wextra -Werror -Wno-unused-parameter

pluginsdir   = $(KLISHLIBDIR)/plugins
plugins_LTLIBRARIES = kplugin-infix.la

kplugin_infix_la_CFLAGS  = $(sysrepo_CFLAGS) $(klish_CFLAGS) $(libyang_CFLAGS) $(CFLAGS)
kplugin_infix_la_LIBADD  = $(sysrepo_LIBS)   $(klish_LIBS)   $(libyang_LIBS)
kplugin_infix_la_LDFLAGS = -module -avoid-version -shared
kplugin_infix_la_SOURCES = infix.c
