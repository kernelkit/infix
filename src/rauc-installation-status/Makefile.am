AM_CPPFLAGS             = -D_DEFAULT_SOURCE -D_XOPEN_SOURCE -D_GNU_SOURCE
bin_PROGRAMS = rauc-installation-status
rauc_installation_status_SOURCES = rauc-installation-status.c

rauc_installation_status_CFLAGS =			\
		$(glib_CFLAGS)				\
		$(jansson_CFLAGS)			\
		$(CFLAGS)
rauc_installation_status_LDADD = 			\
		$(glib_LIBS)				\
		$(jansson_LIBS)

BUILT_SOURCES = $(rauc_installer_sources)
EXTRA_DIST    = de.pengutronix.rauc.Installer.xml

rauc_installer_sources = rauc-installer.c rauc-installer.h
$(rauc_installer_sources): de.pengutronix.rauc.Installer.xml
	$(AM_V_GEN) gdbus-codegen				\
		--generate-c-code rauc-installer		\
		--c-generate-autocleanup all			\
		--c-namespace Rauc				\
		--interface-prefix de.pengutronix.rauc.		\
		$<

nodist_rauc_installation_status_SOURCES = $(rauc_installer_sources)
