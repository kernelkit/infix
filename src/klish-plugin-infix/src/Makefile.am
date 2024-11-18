AM_CPPFLAGS  = -D_DEFAULT_SOURCE -D_XOPEN_SOURCE -D_GNU_SOURCE
CFLAGS       = -Wall -Wextra -Werror -Wno-unused-parameter

lib_LTLIBRARIES = libklish-plugin-infix.la

libklish_plugin_infix_la_CFLAGS  = $(sysrepo_CFLAGS) $(klish_CFLAGS) $(libyang_CFLAGS) $(CFLAGS)
libklish_plugin_infix_la_LIBADD  = $(sysrepo_LIBS)   $(klish_LIBS)   $(libyang_LIBS)
libklish_plugin_infix_la_LDFLAGS = -module -avoid-version -shared
libklish_plugin_infix_la_SOURCES = infix.c
