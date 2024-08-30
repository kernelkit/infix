AM_CPPFLAGS             = -D_DEFAULT_SOURCE -D_XOPEN_SOURCE -D_GNU_SOURCE
plugindir               = $(srpdplugindir)
plugin_LTLIBRARIES      = test-mode-plugin.la

test_mode_plugin_la_LDFLAGS = -module -avoid-version -shared

test_mode_plugin_la_CFLAGS  =			\
	$(sysrepo_CFLAGS)			\
	$(libite_CFLAGS)			\
	$(CFLAGS)

test_mode_plugin_la_LIBADD  =			\
	$(sysrepo_LIBS)				\
	$(libite_LIBS)

test_mode_plugin_la_SOURCES = test-mode.c
