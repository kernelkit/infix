AM_CPPFLAGS             = -DYANG_PATH_=\"$(YANGDIR)/\"

plugindir               = $(srpdplugindir)
plugin_LTLIBRARIES      = confd-plugin.la

confd_plugin_la_CFLAGS  = $(sysrepo_CFLAGS) $(CFLAGS)
confd_plugin_la_LIBADD  = $(sysrepo_LIBS)   $(CFLAGS)
confd_plugin_la_LDFLAGS = -module -avoid-version -shared
confd_plugin_la_SOURCES = ietf-system.c
