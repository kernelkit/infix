include $(top_srcdir)/common.am

clispec_DATA += core-cli.cli

cli_LTLIBRARIES += core-cli.la
core_cli_la_LDFLAGS = $(cli_ldflags)
core_cli_la_LIBADD  = $(cli_libadd)
core_cli_la_SOURCES = core-cli.c

backend_LTLIBRARIES += core-backend.la
core_backend_la_CFLAGS  = $(backend_cflags)
core_backend_la_LDFLAGS = $(backend_ldflags)
core_backend_la_LIBADD  = $(backend_libadd)
core_backend_la_SOURCES = core-backend.c
