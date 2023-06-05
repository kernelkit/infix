CFLAGS                 += -Wall -Wextra -Werror -Wno-unused-parameter
AM_CPPFLAGS             = -D_DEFAULT_SOURCE -D_XOPEN_SOURCE -D_GNU_SOURCE -DYANG_PATH_=\"$(YANGDIR)\"

plugindir               = $(srpdplugindir)
plugin_LTLIBRARIES      = statd-plugin.la

statd_plugin_la_CFLAGS  = $(augeas_CFLAGS) $(jansson_CFLAGS) $(libite_CFLAGS) $(sysrepo_CFLAGS) $(CFLAGS)
statd_plugin_la_LIBADD  = $(augeas_LIBS)   $(jansson_LIBS)   $(libite_LIBS)   $(sysrepo_LIBS)
statd_plugin_la_LDFLAGS = -module -avoid-version -shared
statd_plugin_la_SOURCES = core.c ../lib/common.h \
			  ../lib/lyx.h ../lib/lyx.c \
			  ../lib/srx_module.h ../lib/srx_module.c
