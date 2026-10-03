pkglibexec_SCRIPTS = gen-config gen-hostname gen-interfaces gen-motd gen-hardware \
		     gen-version mstpd-wait-online wait-interface
pkglibexec_PROGRAMS = pppmon
pppmon_SOURCES     = pppmon.c
pppmon_CFLAGS      = $(libite_CFLAGS)
pppmon_LDADD       = $(libite_LIBS)
sbin_SCRIPTS       = dagger migrate firewall
