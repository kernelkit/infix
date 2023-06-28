CFLAGS                 += -Wall -Wextra -Werror -Wno-unused-parameter
AM_CPPFLAGS             = -D_DEFAULT_SOURCE -D_XOPEN_SOURCE -D_GNU_SOURCE -DYANG_PATH_=\"$(YANGDIR)\"

plugindir               = $(srpdplugindir)
plugin_LTLIBRARIES      = confd-plugin.la

confd_plugin_la_CFLAGS  = $(augeas_CFLAGS) $(jansson_CFLAGS) $(libite_CFLAGS) $(sysrepo_CFLAGS) $(CFLAGS)
confd_plugin_la_LIBADD  = $(augeas_LIBS)   $(jansson_LIBS)   $(libite_LIBS)   $(sysrepo_LIBS)
confd_plugin_la_LDFLAGS = -module -avoid-version -shared
confd_plugin_la_SOURCES = core.c core.h ../lib/helpers.c ../lib/helpers.h srx_val.c srx_val.h \
			  ietf-system.c ietf-interfaces.c systemv.c systemv.h ../lib/lyx.c ../lib/lyx.h dagger.c dagger.h \
			  infix-dhcp.c infix-factory.c ../lib/srx_module.c ../lib/srx_module.h \
			  ../lib/common.h
