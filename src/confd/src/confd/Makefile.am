CFLAGS                 += -Wall -Wextra -Werror -Wno-unused-parameter
AM_CPPFLAGS             = -D_DEFAULT_SOURCE -D_XOPEN_SOURCE -D_GNU_SOURCE -DYANG_PATH_=\"$(YANGDIR)\"

plugindir               = $(srpdplugindir)
plugin_LTLIBRARIES      = confd-plugin.la

confd_plugin_la_LDFLAGS = -module -avoid-version -shared

confd_plugin_la_CFLAGS  =			\
	$(augeas_CFLAGS)			\
	$(glib_CFLAGS)				\
	$(jansson_CFLAGS)			\
	$(libite_CFLAGS)			\
	$(sysrepo_CFLAGS)			\
	$(libsrx_CFLAGS)			\
	$(CFLAGS)

confd_plugin_la_LIBADD  =			\
	$(augeas_LIBS)				\
	$(glib_LIBS)				\
	$(jansson_LIBS)				\
	$(libite_LIBS)				\
	$(sysrepo_LIBS)				\
	$(libsrx_LIBS)

rauc_installer_sources = \
	rauc-installer.c \
	rauc-installer.h

BUILT_SOURCES = $(rauc_installer_sources)
EXTRA_DIST = ../lib/de.pengutronix.rauc.Installer.xml

$(rauc_installer_sources): ../lib/de.pengutronix.rauc.Installer.xml
	$(AM_V_GEN) gdbus-codegen			\
		--generate-c-code rauc-installer	\
		--c-generate-autocleanup all		\
		--c-namespace Rauc			\
		--interface-prefix de.pengutronix.rauc.	\
		$<

nodist_confd_plugin_la_SOURCES = \
	$(rauc_installer_sources)

confd_plugin_la_SOURCES =			\
	ietf-interfaces.c			\
	ietf-system.c				\
	infix-dhcp.c				\
	infix-factory.c				\
	infix-system-software.c			\
	core.c		core.h			\
	dagger.c	dagger.h
