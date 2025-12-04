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
	interfaces.c interfaces.h		\
	  ieee802-ethernet-interface.c		\
	  ip.c					\
	  if-bridge.c				\
	  if-lag.c				\
	  if-bridge-mcd.c			\
	  if-bridge-port.c			\
	  if-veth.c				\
	  if-vlan.c				\
	  if-gre.c				\
	  if-vxlan.c				\
	  if-wifi.c				\
	keystore.c				\
	system.c				\
	ntp.c					\
	syslog.c				\
	factory-default.c			\
	routing.c				\
	dhcp-common.c				\
	dhcp-client.c				\
	dhcpv6-client.c				\
	dhcp-server.c				\
	factory.c				\
	firewall.c				\
	meta.c					\
	services.c				\
	system-software.c 			\
	hardware.c

if CONTAINERS
confd_plugin_la_SOURCES += containers.c cni.c cni.h
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
