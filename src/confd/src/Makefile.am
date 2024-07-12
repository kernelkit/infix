AM_CPPFLAGS             = -D_DEFAULT_SOURCE -D_XOPEN_SOURCE -D_GNU_SOURCE -DYANG_PATH_=\"$(YANGDIR)\"
CLEANFILES              = $(rauc_installer_sources)
plugindir               = $(srpdplugindir)
plugin_LTLIBRARIES      = confd-plugin.la

confd_plugin_la_LDFLAGS = -module -avoid-version -shared

confd_plugin_la_CFLAGS  =			\
	$(crypt_CFLAGS)				\
	$(glib_CFLAGS)				\
	$(jansson_CFLAGS)			\
	$(libite_CFLAGS)			\
	$(sysrepo_CFLAGS)			\
	$(libsrx_CFLAGS)			\
	$(CFLAGS)

confd_plugin_la_LIBADD  =			\
	$(crypt_LIBS)				\
	$(glib_LIBS)				\
	$(jansson_LIBS)				\
	$(libite_LIBS)				\
	$(sysrepo_LIBS)				\
	$(libsrx_LIBS)

confd_plugin_la_SOURCES =			\
	base64.c		base64.h	\
	core.c			core.h		\
	dagger.c		dagger.h	\
	ietf-interfaces.c			\
	ietf-keystore.c				\
	ietf-system.c				\
	ietf-syslog.c				\
	ietf-factory-default.c			\
	ietf-routing.c				\
	infix-dhcp.c				\
	infix-factory.c				\
	infix-meta.c				\
	infix-services.c			\
	infix-system-software.c 		\
	ietf-hardware.c

if CONTAINERS
confd_plugin_la_SOURCES += infix-containers.c cni.c cni.h
endif

rauc_installer_sources = 			\
	rauc-installer.c rauc-installer.h

BUILT_SOURCES = $(rauc_installer_sources)
EXTRA_DIST    = de.pengutronix.rauc.Installer.xml

$(rauc_installer_sources): de.pengutronix.rauc.Installer.xml
	$(AM_V_GEN) gdbus-codegen				\
		--generate-c-code rauc-installer		\
		--c-generate-autocleanup all			\
		--c-namespace Rauc				\
		--interface-prefix de.pengutronix.rauc.		\
		$<

nodist_confd_plugin_la_SOURCES = $(rauc_installer_sources)
