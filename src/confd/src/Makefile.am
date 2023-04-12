AM_CPPFLAGS             = -D_DEFAULT_SOURCE -D_XOPEN_SOURCE -DYANG_PATH_=\"$(YANGDIR)/\"

plugindir               = $(srpdplugindir)
plugin_LTLIBRARIES      = confd-plugin.la

confd_plugin_la_CFLAGS  = $(augeas_CFLAGS) $(sysrepo_CFLAGS) $(CFLAGS)
confd_plugin_la_LIBADD  = $(augeas_LIBS)   $(sysrepo_LIBS)   $(CFLAGS)
confd_plugin_la_LDFLAGS = -module -avoid-version -shared
confd_plugin_la_SOURCES = core.c core.h run.c ietf-system.c ietf-interfaces.c
