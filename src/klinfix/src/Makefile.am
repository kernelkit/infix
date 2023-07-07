CFLAGS   = -Wall -Wextra -Werror -Wno-unused-parameter

pluginsdir   = $(KLISHLIBDIR)/plugins
plugins_LTLIBRARIES = kplugin-klinfix.la

kplugin_klinfix_la_CFLAGS  = $(sysrepo_CFLAGS) $(klish_CFLAGS) $(libyang_CFLAGS) $(CFLAGS)
kplugin_klinfix_la_LIBADD  = $(sysrepo_LIBS)   $(klish_LIBS)   $(libyang_LIBS)
kplugin_klinfix_la_LDFLAGS = -module -avoid-version -shared
kplugin_klinfix_la_SOURCES = klinfix.c
